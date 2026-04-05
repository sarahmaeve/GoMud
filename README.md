# GoMud

![image](feature-screenshots/splash.png)

GoMud is an in-development open source MUD (Multi-user Dungeon) game world and library.

It ships with a default world to play in, but can be overwritten or modified to build your own world using built-in tools.

# User Support

If you have comments, questions, suggestions:

[Github Discussions](https://github.com/GoMudEngine/GoMud/discussions) - Don't be shy. Your questions or requests might help others too.

[Discord Server](https://discord.gg/cjukKvQWyy) - Get more interactive help in the GoMud Discord server.

[Guides](_datafiles/guides/README.md) - Community created guides to help get started.

# Contributor Guide

Interested in contributing? Check out our [CONTRIBUTING.md](https://github.com/GoMudEngine/GoMud/blob/master/.github/CONTRIBUTING.md) to learn about the process.

## Screenshots

Click below to see in-game screenshots of just a handful of features:

[![Feature Screenshots](feature-screenshots/screenshots-thumb.png "Feature Screenshots")](feature-screenshots/README.md)

## ANSI Colors

Colorization is handled through extensive use of my [github.com/GoMudEngine/ansitags](https://github.com/GoMudEngine/ansitags) library.

## Small Feature Demos

- [Auto-complete input](https://youtu.be/7sG-FFHdhtI)
- [In-game maps](https://youtu.be/navCCH-mz_8)
- [Quests / Quest Progress](https://youtu.be/3zIClk3ewTU)
- [Lockpicking](https://youtu.be/-zgw99oI0XY)
- [Hired Mercs](https://youtu.be/semi97yokZE)
- [TinyMap](https://www.youtube.com/watch?v=VLNF5oM4pWw) (okay not much of a "feature")
- [256 Color/xterm](https://www.youtube.com/watch?v=gGSrLwdVZZQ)
- [Customizable Prompts](https://www.youtube.com/watch?v=MFkmjSTL0Ds)
- [Mob/NPC Scripting](https://www.youtube.com/watch?v=li2k1N4p74o)
- [Room Scripting](https://www.youtube.com/watch?v=n1qNUjhyOqg)
- [Kill Stats](https://www.youtube.com/watch?v=4aXs8JNj5Cc)
- [Searchable Inventory](https://www.youtube.com/watch?v=iDUbdeR2BUg)
- [Day/Night Cycles](https://www.youtube.com/watch?v=CiEbOp244cw)
- [Web Socket "Virtual Terminal"](https://www.youtube.com/watch?v=L-qtybXO4aw)
- [Alternate Characters](https://www.youtube.com/watch?v=VERF2l70W34)

## Connecting

_TELNET_ : connect to `localhost` on port `33333` with a telnet client

_WEB CLIENT_: [http://localhost/webclient](http://localhost/webclient)

The first time you run the server, you must initialize the persistence database and create an admin account (see the [Persistence](#persistence) section below). There is no longer a baked-in default account — you pick your own admin credentials at first boot.

## Persistence

GoMud uses a hybrid persistence model:

- **Content templates** (rooms, mobs, items, quests, spells, races, buffs) live as YAML files in `<data-dir>/world/<worldname>/`. Content creators edit these directly in a text editor or via in-game OLC commands (`#build`, `#room edit`, etc.).
- **Runtime state** (player records, room instance overlays, auction state) lives in a SQLite database at `<data-dir>/db/<worldname>_mud.db` by default. Writes are buffered through a background worker and committed in batches, so game logic never blocks on disk I/O.

### First-time setup

On first run you must explicitly initialize the database. The server will refuse to start with a missing database file unless `--init-db` is supplied — this prevents a misconfigured path from silently creating an empty database alongside a real one.

```
./GoMud --init-db --create-admin "youradminname:yourpassword"
```

The `--create-admin` flag takes a single `username:password` argument. Both sides are validated against the project's existing rules (username length, banned-name patterns, mob-name collisions; password length 8-24 by default). The password is hashed with bcrypt before being stored — it is not kept in plaintext.

After the first run, subsequent starts do not need `--init-db`:

```
./GoMud
```

### Running the binary from a different directory

By default the binary expects a `_datafiles/` directory in the current working directory. You can override this with `--data-dir`:

```
./GoMud --data-dir /opt/gomud/data
./GoMud --data-dir ~/my-mud --init-db
```

The data directory contains `config.yaml`, `world/<worldname>/` content, `localize/`, `sample-scripts/`, and `db/`. You can also point at an alternate config file explicitly with `--config`:

```
./GoMud --data-dir /opt/gomud/data --config /etc/gomud/production.yaml
```

Relative paths in the config file (e.g., `FilePaths.DataFiles: world/default`) resolve against the data directory. Absolute paths pass through unchanged.

### Command-line flags

| Flag | Description |
|---|---|
| `--data-dir <path>` | Base data directory (default: `./_datafiles`) |
| `--config <path>` | Config file path (default: `<data-dir>/config.yaml`) |
| `--init-db` | Create the persistence database if it does not exist |
| `--create-admin <username:password>` | Create an admin account on `--init-db`. Requires `--init-db`. |
| `--version` | Print version and exit |
| `--port-search <min-max>` | Find available ports in a range and exit |

### Inspecting the database

The database is a standard SQLite file. You can open it with any SQLite tool:

```
sqlite3 _datafiles/db/default_mud.db
sqlite> SELECT user_id, username, role FROM users;
sqlite> .schema
```

### Backups

Stop the server cleanly (so the write-ahead log is committed), then copy the `.db`, `.db-shm`, and `.db-wal` files as a set. For live backups without downtime, use SQLite's `.backup` command or a tool like LiteStream.

## Env Vars

When running several environment variables can be set to alter behaviors of the mud:

- **CONFIG_PATH**_=/path/to/alternative/config.yaml_ - This can provide a path to a copy of the config.yaml containing only values you wish to override. This way you don't have to modify the original config.yaml
- **LOG_PATH**_=/path/to/log.txt_ - This will write all logs to a specified file. If unspecified, will write to _stderr_.
- **LOG_LEVEL**_={LOW/MEDIUM/HIGH}_ - This sets how verbose you want the logs to be. _(Note: Log files rotate every 100MB)_
- **LOG_NOCOLOR**_=1_ - If set, logs will be written without colorization.

# Why Go?

Why not?

Go provides a lot of terrific benefits such as:

- Compatible - High degree of compatibility across platforms or CPU Architectures. Go code quite painlessly compiles for Windows, Linux, ARM, etc. with minimal to no changes to the code.
- Fast - Go is fast. From execution to builds. The current GoMud project builds on a Macbook in less than a couple of seconds.
- Opinionated - Go style and patterns are well established and provide a reliable way to dive into a project and immediately feel familiar with the style.
- Modern - Go is a relatively new/modern language without the burden of "every feature people thought would be useful in the last 30 or 40 years" added to it.
- Upgradable - Go's promise of maintaining backward compatibility means upgrading versions over time remains a simple and painless process (If not downright invisible).
- Statically Linked - If you have the binary, you have the working program. Externally linked dependencies (and whether you have them) are not an issue.
- No Central Registries - Go is built to naturally incorporate library includes straight from their repos (such as git). This is neato.
- Concurrent - Go has concurrency built in as a feature of the language, not a library you include.
