# mini-redis

A small clone of Redis written in Go, from scratch, using only the standard
library. It speaks the real RESP protocol over TCP, so the actual
`redis-cli` can connect to it and run commands against it.

This was built as a learning project to understand how Redis works under
the hood: TCP servers, the RESP wire protocol, concurrency with goroutines,
and key expiry.

## What it supports

- `PING`, `ECHO`
- `SET` (with optional `EX seconds`), `GET`
- `DEL`, `EXISTS`, `KEYS`
- `TTL`, `EXPIRE`
- `INCR`, `DECR`
- `COMMAND DOCS` (so `redis-cli` doesn't complain on connect)
- Key expiry, both lazy (checked when a key is read) and active (a
  background goroutine sweeps expired keys every second)
- Concurrent clients, each handled in its own goroutine
- Graceful shutdown on Ctrl+C
- Connect/disconnect logging per client, and a recovered panic in one
  connection's handler won't take down the other clients or the server

## Running it

You need Go installed (1.21+ is fine). No other dependencies.

```bash
go run .
```

By default it listens on `localhost:6379`, the same port real Redis uses.
Press Ctrl+C to shut it down cleanly.

You can also build a binary:

```bash
go build -o mini-redis .
./mini-redis
```

## Trying it out with redis-cli

In another terminal, connect with the real `redis-cli`:

```
$ redis-cli
127.0.0.1:6379> PING
PONG
127.0.0.1:6379> SET name akshat
OK
127.0.0.1:6379> GET name
"akshat"
127.0.0.1:6379> SET session abc123 EX 30
OK
127.0.0.1:6379> TTL session
(integer) 30
127.0.0.1:6379> EXISTS name session missing
(integer) 2
127.0.0.1:6379> KEYS *
1) "name"
2) "session"
127.0.0.1:6379> SET counter 10
OK
127.0.0.1:6379> INCR counter
(integer) 11
127.0.0.1:6379> DECR counter
(integer) 10
127.0.0.1:6379> DEL counter
(integer) 1
127.0.0.1:6379> GET counter
(nil)
127.0.0.1:6379> EXPIRE name 60
(integer) 1
127.0.0.1:6379> TTL name
(integer) 60
```

## Project layout

```
main.go        entry point, starts the store, sweeper, and TCP server, handles Ctrl+C
server.go      TCP listener and the per-connection loop
resp.go        RESP protocol parsing (reading commands) and writing (replies)
store.go       the actual key-value map, mutex locking, and expiry logic
commands.go    turns a parsed command into a call against the store and a reply
```

## Notes

- Everything lives in memory. Nothing is persisted to disk, so restarting
  the server clears all data.
- This is a learning project, not something you'd run in production. It
  doesn't implement most of real Redis (data types beyond strings,
  replication, persistence, pub/sub, etc).
