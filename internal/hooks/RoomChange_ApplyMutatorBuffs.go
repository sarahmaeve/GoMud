package hooks

import (
	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/rooms"
	"github.com/GoMudEngine/GoMud/internal/users"
)

// ApplyMutatorBuffs applies zone/room mutator buffs to a player when
// they enter a new room. Registered with events.First so it runs
// before other RoomChange listeners, preserving the original ordering
// where MoveToRoom applied buffs before firing the event.
func ApplyMutatorBuffs(e events.Event) events.ListenerReturn {
	evt, typeOk := e.(events.RoomChange)
	if !typeOk {
		mudlog.Error("ApplyMutatorBuffs", "Expected Type", "RoomChange", "Actual Type", e.Type())
		return events.Cancel
	}

	// Only applies to players, not mobs.
	if evt.UserId == 0 {
		return events.Continue
	}

	user := users.GetByUserId(evt.UserId)
	if user == nil {
		return events.Continue
	}

	newRoom := rooms.LoadRoom(evt.ToRoomId)
	if newRoom == nil {
		return events.Continue
	}

	for mut := range newRoom.ActiveMutators {
		spec := mut.GetSpec()
		if len(spec.PlayerBuffIds) == 0 {
			continue
		}
		for _, buffId := range spec.PlayerBuffIds {
			if !user.Character.HasBuff(buffId) {
				user.AddBuff(buffId, "area")
			}
		}
	}

	return events.Continue
}
