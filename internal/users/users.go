package users

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GoMudEngine/GoMud/internal/characters"
	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/connections"
	"github.com/GoMudEngine/GoMud/internal/mobs"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/persistence"
	"github.com/GoMudEngine/GoMud/internal/util"
)

var (
	userManager *ActiveUsers = newUserManager()

	// createUserMu serializes the CreateUser path end-to-end. Without
	// this, two concurrent CreateUser calls would each compute the same
	// "next" user id via GetUniqueUserId (because the async store write
	// from the first call hasn't landed yet), issue two SaveUsers with
	// the same id (last-writer-wins in the coalesced batch), and clobber
	// each other in userManager.Users. The mutex is narrowly scoped to
	// the CreateUser path — it does not block login, save, or any
	// read-only operations (H3).
	createUserMu sync.Mutex
)

type ActiveUsers struct {
	mu                sync.RWMutex                        // guards all maps below
	Users             map[int]*UserRecord                 // userId to UserRecord
	Usernames         map[string]int                      // username to userId
	Connections       map[connections.ConnectionId]int    // connectionId to userId
	UserConnections   map[int]connections.ConnectionId    // userId to connectionId
	ZombieConnections map[connections.ConnectionId]uint64 // connectionId to turn they became a zombie
}

func newUserManager() *ActiveUsers {
	return &ActiveUsers{
		Users:             make(map[int]*UserRecord),
		Usernames:         make(map[string]int),
		Connections:       make(map[connections.ConnectionId]int),
		UserConnections:   make(map[int]connections.ConnectionId),
		ZombieConnections: make(map[connections.ConnectionId]uint64),
	}
}

func RemoveZombieUser(userId int) {
	userManager.mu.Lock()
	defer userManager.mu.Unlock()

	if u := userManager.Users[userId]; u != nil {
		u.Character.SetAdjective(`zombie`, false)
	}
	if connId, ok := userManager.UserConnections[userId]; ok {
		delete(userManager.ZombieConnections, connId)
	}
}

func IsZombieConnection(connectionId connections.ConnectionId) bool {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()
	_, ok := userManager.ZombieConnections[connectionId]
	return ok
}

func RemoveZombieConnection(connectionId connections.ConnectionId) {
	userManager.mu.Lock()
	defer userManager.mu.Unlock()
	delete(userManager.ZombieConnections, connectionId)
}

// Returns a slice of userId's
// These userId's are zombies that have reached expiration
func GetExpiredZombies(expirationTurn uint64) []int {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()

	expiredUsers := make([]int, 0)

	for connectionId, zombieTurn := range userManager.ZombieConnections {
		if zombieTurn < expirationTurn {
			expiredUsers = append(expiredUsers, userManager.Connections[connectionId])
		}
	}

	return expiredUsers
}

func GetConnectionId(userId int) connections.ConnectionId {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()
	if user, ok := userManager.Users[userId]; ok {
		return user.connectionId
	}
	return 0
}

func GetConnectionIds(userIds []int) []connections.ConnectionId {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()

	connectionIds := make([]connections.ConnectionId, 0, len(userIds))
	for _, userId := range userIds {
		if user, ok := userManager.Users[userId]; ok {
			connectionIds = append(connectionIds, user.connectionId)
		}
	}

	return connectionIds
}

func GetAllActiveUsers() []*UserRecord {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()

	ret := []*UserRecord{}

	for _, userPtr := range userManager.Users {
		if !userPtr.isZombie {
			ret = append(ret, userPtr)
		}
	}

	return ret
}

func GetOnlineUserIds() []int {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()

	onlineList := make([]int, 0, len(userManager.Users))
	for _, user := range userManager.Users {
		onlineList = append(onlineList, user.UserId)
	}
	return onlineList
}

func GetByCharacterName(name string) *UserRecord {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()

	var closeMatch *UserRecord = nil

	name = strings.ToLower(name)
	for _, user := range userManager.Users {
		testName := strings.ToLower(user.Character.Name)
		if testName == name {
			return user
		}
		if strings.HasPrefix(testName, name) {
			closeMatch = user
		}
	}

	return closeMatch
}

// GetByUserId looks up an active user by their user ID.
// Returns nil if the user is not currently online.
func (a *ActiveUsers) GetByUserId(userId int) *UserRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if user, ok := a.Users[userId]; ok {
		return user
	}

	return nil
}

// GetByUserId is a package-level convenience that delegates to the
// global userManager singleton.
func GetByUserId(userId int) *UserRecord {
	return userManager.GetByUserId(userId)
}

func GetByConnectionId(connectionId connections.ConnectionId) *UserRecord {
	userManager.mu.RLock()
	defer userManager.mu.RUnlock()

	if userId, ok := userManager.Connections[connectionId]; ok {
		return userManager.Users[userId]
	}

	return nil
}

// First time creating a user.
func LoginUser(user *UserRecord, connectionId connections.ConnectionId) (*UserRecord, string, error) {

	mudlog.Info("LoginUser()", "username", user.Username, "connectionId", connectionId)

	user.Character.SetAdjective(`zombie`, false)

	userManager.mu.Lock()

	// If they're already logged in
	if userId, ok := userManager.Usernames[user.Username]; ok {

		// Do they have a connection tracked?
		if otherConnId, ok := userManager.UserConnections[userId]; ok {

			// Is it a zombie connection? If so, lets make this new connection the owner
			if _, isZombie := userManager.ZombieConnections[otherConnId]; isZombie {

				mudlog.Info("LoginUser()", "Zombie", true)

				if zombieUser, ok := userManager.Users[user.UserId]; ok {
					user = zombieUser
				}

				// inline RemoveZombieConnection — already holding mu
				delete(userManager.ZombieConnections, otherConnId)

				user.connectionId = connectionId

				userManager.Users[user.UserId] = user
				userManager.Usernames[user.Username] = user.UserId
				userManager.Connections[user.connectionId] = user.UserId
				userManager.UserConnections[user.UserId] = user.connectionId

				userManager.mu.Unlock()

				for _, mobInstId := range user.Character.GetCharmIds() {
					if !mobs.MobInstanceExists(mobInstId) {
						user.Character.TrackCharmed(mobInstId, false)
					}
				}

				// Set their input round to current to track idle time fresh
				user.SetLastInputRound(util.GetRoundCount())

				user.EventLog.Add(`conn`, `Reconnected`)

				return user, "Reconnecting...", nil
			}

		}

		userManager.mu.Unlock()
		// Otherwise, someone else is logged in, can't double-login!
		return nil, "That user is already logged in.", errors.New("user is already logged in")
	}

	mudlog.Info("LoginUser()", "Zombie", false)

	// Set their input round to current to track idle time fresh
	user.SetLastInputRound(util.GetRoundCount())

	user.connectionId = connectionId

	userManager.Users[user.UserId] = user
	userManager.Usernames[user.Username] = user.UserId
	userManager.Connections[user.connectionId] = user.UserId
	userManager.UserConnections[user.UserId] = user.connectionId

	userManager.mu.Unlock()

	mudlog.Info("LOGIN", "userId", user.UserId)

	user.EventLog.Add(`conn`, `Connected`)

	for _, mobInstId := range user.Character.GetCharmIds() {
		if !mobs.MobInstanceExists(mobInstId) {
			user.Character.TrackCharmed(mobInstId, false)
		}
	}

	return user, "", nil
}

func SetZombieUser(userId int) {
	userManager.mu.Lock()
	defer userManager.mu.Unlock()

	if u, ok := userManager.Users[userId]; ok {

		u.Character.RemoveBuff(0)
		u.Character.SetAdjective(`zombie`, true)

		// Prevent guide mob dupes
		for _, miid := range u.Character.CharmedMobs {
			if m := mobs.GetInstance(miid); m != nil {
				if m.MobId == 38 {
					m.Character.Charmed.RoundsRemaining = 0
				}
			}
		}

		if _, ok := userManager.ZombieConnections[u.connectionId]; ok {
			return
		}

		userManager.ZombieConnections[u.connectionId] = util.GetTurnCount()
	}

}

// SaveAllUsers enqueues a write for every currently active user. Writes
// are asynchronous — the background store worker commits them in batches.
// Call persistence.Store.Flush() (via GetStore().Flush()) if you need
// read-after-write consistency.
func SaveAllUsers(isAutoSave ...bool) {
	// Snapshot user records under read lock so we don't hold mu across
	// the (short) serialization path.
	userManager.mu.RLock()
	snapshot := make([]UserRecord, 0, len(userManager.Users))
	for _, u := range userManager.Users {
		snapshot = append(snapshot, *u)
	}
	userManager.mu.RUnlock()

	for _, u := range snapshot {
		if err := SaveUser(u, isAutoSave...); err != nil {
			mudlog.Error("SaveAllUsers()", "error", err.Error())
		}
	}
}

func LogOutUserByConnectionId(connectionId connections.ConnectionId) error {

	userManager.mu.Lock()

	userId, connExists := userManager.Connections[connectionId]
	if !connExists {
		userManager.mu.Unlock()
		return errors.New("user not found for connection")
	}

	u := userManager.Users[userId]

	if u != nil {
		// Snapshot before releasing lock so we can do I/O outside critical section.
		uCopy := *u
		delete(userManager.Users, u.UserId)
		delete(userManager.Usernames, u.Username)
		delete(userManager.Connections, u.connectionId)
		delete(userManager.UserConnections, u.UserId)
		userManager.mu.Unlock()

		// Make sure the user data is saved to a file (I/O outside lock).
		uCopy.Character.Validate()
		SaveUser(uCopy)
	} else {
		// Connection exists but user record is missing — clean up the connection entry.
		delete(userManager.Connections, connectionId)
		userManager.mu.Unlock()
	}

	return nil
}

// CreateUser registers a brand-new user in the persistence store and
// adds them to the in-memory active user map. The caller must have
// already set the Password (via SetPassword, which hashes with bcrypt).
func CreateUser(u *UserRecord) error {

	if err := ValidateName(u.Username); err != nil {
		return errors.New("that username is not allowed: " + err.Error())
	}

	// Serialize the entire create path. GetUniqueUserId computes the
	// next id from the store + in-memory active users; a concurrent
	// CreateUser would otherwise race the allocation and hand out the
	// same id twice (H3).
	createUserMu.Lock()
	defer createUserMu.Unlock()

	// Re-check existence under the lock so two concurrent signups for
	// the same username don't both succeed (ValidateName's Exists check
	// above races the same way as the id allocation did).
	if Exists(u.Username) {
		return errors.New("that username is already in use")
	}

	// GetUniqueUserId reads from the store — call before acquiring mu.
	u.UserId = GetUniqueUserId()
	// Default to RoleUser if the caller hasn't set one. Don't clobber a
	// role the caller explicitly assigned — createAdminUser relies on
	// being able to insert the admin row with RoleAdmin in a single
	// write, so a crash between insert and promotion cannot leave the
	// bootstrap admin as a regular user.
	if u.Role == "" {
		u.Role = RoleUser
	}

	// Persist first so a crash before the in-memory insert doesn't leave
	// a user dangling in RAM without durability. SaveUser is async; the
	// write enqueues and commits within the next batch window.
	if err := SaveUser(*u); err != nil {
		return err
	}

	userManager.mu.Lock()
	userManager.Users[u.UserId] = u
	userManager.Usernames[u.Username] = u.UserId
	userManager.Connections[u.connectionId] = u.UserId
	userManager.UserConnections[u.UserId] = u.connectionId
	userManager.mu.Unlock()

	return nil
}

// LoadUser loads a user from the persistence store by username
// (case-insensitive). Returns an error if the store is not initialized
// or the user doesn't exist.
func LoadUser(username string, skipValidation ...bool) (*UserRecord, error) {
	if err := requireStore(); err != nil {
		return nil, err
	}

	data, err := store.LoadUserByUsername(username)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, errors.New("user doesn't exist")
		}
		return nil, fmt.Errorf("LoadUser: %w", err)
	}

	loadedUser, err := dataToUserRecord(data)
	if err != nil {
		mudlog.Error("LoadUser", "error", err.Error())
		return nil, fmt.Errorf("LoadUser unmarshal: %w", err)
	}

	if len(skipValidation) == 0 || !skipValidation[0] {
		if err := loadedUser.Character.Validate(true); err == nil {
			SaveUser(*loadedUser)
		}
	}

	if loadedUser.Joined.IsZero() {
		loadedUser.Joined = time.Now()
	}

	// Set their connection time to now
	loadedUser.connectionTime = time.Now()

	return loadedUser, nil
}

// SearchOfflineUsers iterates every user in the persistence store that
// is NOT currently active (logged in) and invokes searchFunc on each.
// Stops early if searchFunc returns false.
func SearchOfflineUsers(searchFunc func(u *UserRecord) bool) {
	if err := requireStore(); err != nil {
		mudlog.Error("SearchOfflineUsers", "error", err.Error())
		return
	}

	// Snapshot online usernames so we can filter quickly without
	// holding userManager.mu during the iteration.
	userManager.mu.RLock()
	online := make(map[string]struct{}, len(userManager.Usernames))
	for name := range userManager.Usernames {
		online[strings.ToLower(name)] = struct{}{}
	}
	userManager.mu.RUnlock()

	names, err := store.AllUsernames()
	if err != nil {
		mudlog.Error("SearchOfflineUsers: AllUsernames", "error", err.Error())
		return
	}

	for _, name := range names {
		if _, isOnline := online[strings.ToLower(name)]; isOnline {
			continue
		}
		data, err := store.LoadUserByUsername(name)
		if err != nil {
			mudlog.Error("SearchOfflineUsers: LoadUserByUsername", "name", name, "error", err.Error())
			continue
		}
		u, err := dataToUserRecord(data)
		if err != nil {
			mudlog.Error("SearchOfflineUsers: unmarshal", "name", name, "error", err.Error())
			continue
		}
		if !searchFunc(u) {
			return
		}
	}
}

func ValidateName(name string) error {

	validation := configs.GetValidationConfig()

	if len(name) < int(validation.NameSizeMin) || len(name) > int(validation.NameSizeMax) {
		return fmt.Errorf("name must be between %d and %d characters long", validation.NameSizeMin, validation.NameSizeMax)
	}

	if validation.NameRejectRegex != `` {
		if !regexp.MustCompile(validation.NameRejectRegex.String()).MatchString(name) {
			return errors.New(validation.NameRejectReason.String())
		}
	}

	if bannedPattern, ok := configs.GetConfig().IsBannedName(name); ok {
		return errors.New(`that username matched the prohibited name pattern: "` + bannedPattern + `"`)
	}

	for _, mobName := range mobs.GetAllMobNames() {
		if strings.EqualFold(mobName, name) {
			return errors.New("that username is in use")
		}
	}

	if Exists(name) {
		return errors.New("that username is in use")
	}

	return nil
}

func ValidatePassword(pw string) error {

	validation := configs.GetValidationConfig()

	if len(pw) < int(validation.PasswordSizeMin) || len(pw) > int(validation.PasswordSizeMax) {
		return fmt.Errorf("password must be between %d and %d characters long", validation.PasswordSizeMin, validation.PasswordSizeMax)
	}

	return nil
}

// searches for a character name and returns the user that owns it
// Slow and possibly memory intensive - use strategically
func CharacterNameSearch(nameToFind string) (foundUserId int, foundUserName string) {

	foundUserId = 0
	foundUserName = ``

	SearchOfflineUsers(func(u *UserRecord) bool {

		if strings.EqualFold(u.Character.Name, nameToFind) {
			foundUserId = u.UserId
			foundUserName = u.Username
			return false
		}

		// Not found? Search alts...

		for _, char := range characters.LoadAlts(u.UserId) {
			if strings.EqualFold(char.Name, nameToFind) {
				foundUserId = u.UserId
				foundUserName = u.Username
				return false
			}
		}

		return true
	})

	return foundUserId, foundUserName
}

// SaveUser enqueues a write for the user in the persistence store.
// The write is asynchronous — the background worker commits it in the
// next batch. Use GetStore().Flush() to wait for pending writes, or
// rely on graceful shutdown to flush everything via Close().
//
// The isAutoSave variadic is accepted for backward compatibility with
// existing call sites and has no effect on the store backend (the
// worker handles batching regardless of call origin).
func SaveUser(u UserRecord, isAutoSave ...bool) error {
	if err := requireStore(); err != nil {
		mudlog.Error("SaveUser", "username", u.Username, "userId", u.UserId, "error", err.Error())
		return err
	}

	data, err := userRecordToData(&u)
	if err != nil {
		mudlog.Error("SaveUser marshal", "username", u.Username, "userId", u.UserId, "error", err.Error())
		return fmt.Errorf("SaveUser: %w", err)
	}

	// Log errors centrally — C3: many call sites drop SaveUser's return
	// value, so without logging here an enqueue failure (queue saturated,
	// store closed, etc.) would be silent data loss.
	if err := store.SaveUser(data); err != nil {
		mudlog.Error("SaveUser enqueue", "username", u.Username, "userId", u.UserId, "error", err.Error())
		return err
	}
	return nil
}

// GetUniqueUserId returns the next available user id by scanning both
// the in-memory active user map and the persistence store.
func GetUniqueUserId() int {
	highestUserId := 0

	if err := requireStore(); err == nil {
		if ids, err := store.AllUserIds(); err == nil {
			for _, id := range ids {
				if id > highestUserId {
					highestUserId = id
				}
			}
		} else {
			mudlog.Error("GetUniqueUserId: AllUserIds", "error", err.Error())
		}
	}

	// Also check online users in case there's a newly-created user
	// whose store write is still in flight.
	for _, u := range GetAllActiveUsers() {
		if u.UserId > highestUserId {
			highestUserId = u.UserId
		}
	}

	return highestUserId + 1
}

// Exists returns true if a user with the given name is registered,
// either actively online or stored in the persistence store.
func Exists(name string) bool {
	for _, u := range GetAllActiveUsers() {
		if strings.EqualFold(u.Username, name) {
			return true
		}
	}

	if err := requireStore(); err != nil {
		mudlog.Error("Exists: store not initialized", "error", err.Error())
		return false
	}

	exists, err := store.UserExists(name)
	if err != nil {
		mudlog.Error("Exists: store query", "error", err.Error())
		return false
	}
	return exists
}

// FindUserId returns the user id for a given username, or 0 if the
// user doesn't exist.
func FindUserId(username string) int {
	// Check active users first — avoids a store round-trip for the
	// common case (user is already logged in).
	for _, u := range GetAllActiveUsers() {
		if strings.EqualFold(u.Username, username) {
			return u.UserId
		}
	}

	if err := requireStore(); err != nil {
		return 0
	}

	data, err := store.LoadUserByUsername(username)
	if err != nil {
		return 0
	}
	return data.UserId
}
