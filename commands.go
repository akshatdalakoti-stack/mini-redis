package main

import (
	"bufio"
	"path/filepath"
	"strconv"
	"strings"
)

// handleCommand looks at the first word of args and runs whatever it
// matches. Everything gets uppercased first since Redis commands are
// case-insensitive (get, GET, GeT all work).
func handleCommand(store *Store, writer *bufio.Writer, args []string) {
	if len(args) == 0 {
		return
	}

	cmd := strings.ToUpper(args[0])

	if cmd == "PING" {
		handlePing(writer, args)
	} else if cmd == "ECHO" {
		handleEcho(writer, args)
	} else if cmd == "SET" {
		handleSet(store, writer, args)
	} else if cmd == "GET" {
		handleGet(store, writer, args)
	} else if cmd == "DEL" {
		handleDel(store, writer, args)
	} else if cmd == "EXISTS" {
		handleExists(store, writer, args)
	} else if cmd == "KEYS" {
		handleKeys(store, writer, args)
	} else if cmd == "TTL" {
		handleTTL(store, writer, args)
	} else if cmd == "EXPIRE" {
		handleExpire(store, writer, args)
	} else if cmd == "INCR" {
		handleIncr(store, writer, args)
	} else if cmd == "DECR" {
		handleDecr(store, writer, args)
	} else if cmd == "COMMAND" {
		handleCommandDocs(writer, args)
	} else {
		writeError(writer, "ERR unknown command '"+args[0]+"'")
	}
}

func handlePing(writer *bufio.Writer, args []string) {
	if len(args) == 1 {
		writeSimpleString(writer, "PONG")
	} else if len(args) == 2 {
		writeBulkString(writer, args[1])
	} else {
		writeError(writer, "ERR wrong number of arguments for 'ping' command")
	}
}

func handleEcho(writer *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(writer, "ERR wrong number of arguments for 'echo' command")
		return
	}
	writeBulkString(writer, args[1])
}

func handleSet(store *Store, writer *bufio.Writer, args []string) {
	if len(args) == 3 {
		store.Set(args[1], args[2])
		writeSimpleString(writer, "OK")
		return
	}

	if len(args) == 5 && strings.ToUpper(args[3]) == "EX" {
		seconds, err := strconv.Atoi(args[4])
		if err != nil || seconds <= 0 {
			writeError(writer, "ERR invalid expire time in 'set' command")
			return
		}
		store.SetWithTTL(args[1], args[2], seconds)
		writeSimpleString(writer, "OK")
		return
	}

	writeError(writer, "ERR syntax error")
}

func handleGet(store *Store, writer *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(writer, "ERR wrong number of arguments for 'get' command")
		return
	}

	value, ok := store.Get(args[1])
	if !ok {
		writeNullBulkString(writer)
		return
	}
	writeBulkString(writer, value)
}

func handleDel(store *Store, writer *bufio.Writer, args []string) {
	if len(args) < 2 {
		writeError(writer, "ERR wrong number of arguments for 'del' command")
		return
	}
	count := store.Del(args[1:])
	writeInteger(writer, count)
}

func handleExists(store *Store, writer *bufio.Writer, args []string) {
	if len(args) < 2 {
		writeError(writer, "ERR wrong number of arguments for 'exists' command")
		return
	}
	count := store.Exists(args[1:])
	writeInteger(writer, count)
}

func handleKeys(store *Store, writer *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(writer, "ERR wrong number of arguments for 'keys' command")
		return
	}

	pattern := args[1]
	allKeys := store.Keys()

	if pattern == "*" {
		writeArray(writer, allKeys)
		return
	}

	matched := []string{}
	for _, key := range allKeys {
		ok, err := filepath.Match(pattern, key)
		if err == nil && ok {
			matched = append(matched, key)
		}
	}
	writeArray(writer, matched)
}

func handleTTL(store *Store, writer *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(writer, "ERR wrong number of arguments for 'ttl' command")
		return
	}
	writeInteger(writer, store.TTL(args[1]))
}

func handleExpire(store *Store, writer *bufio.Writer, args []string) {
	if len(args) != 3 {
		writeError(writer, "ERR wrong number of arguments for 'expire' command")
		return
	}

	seconds, err := strconv.Atoi(args[2])
	if err != nil {
		writeError(writer, "ERR value is not an integer or out of range")
		return
	}

	writeInteger(writer, store.Expire(args[1], seconds))
}

func handleIncr(store *Store, writer *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(writer, "ERR wrong number of arguments for 'incr' command")
		return
	}

	result, err := store.Incr(args[1])
	if err != nil {
		writeError(writer, "ERR value is not an integer or out of range")
		return
	}
	writeInteger(writer, result)
}

func handleDecr(store *Store, writer *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(writer, "ERR wrong number of arguments for 'decr' command")
		return
	}

	result, err := store.Decr(args[1])
	if err != nil {
		writeError(writer, "ERR value is not an integer or out of range")
		return
	}
	writeInteger(writer, result)
}

// COMMAND DOCS is what redis-cli sends right after connecting to build
// its autocomplete/help info. We don't implement real docs, we just
// reply with an empty array so redis-cli doesn't print an error.
func handleCommandDocs(writer *bufio.Writer, args []string) {
	writeEmptyArray(writer)
}
