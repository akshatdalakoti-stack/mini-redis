package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
)

// RunServer opens the TCP listener and accepts clients until ctx is
// cancelled. Each client gets its own goroutine so multiple redis-cli
// sessions can talk to the server at the same time.
func RunServer(ctx context.Context, store *Store, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Println("mini-redis listening on", address)

	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
			}

			// A temporary network error (e.g. too many open files)
			// shouldn't take the whole server down; log it and keep
			// accepting new connections.
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("accept error (temporary):", err)
				continue
			}
			return err
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConnection(store, conn)
		}()
	}
}

func handleConnection(store *Store, conn net.Conn) {
	remote := conn.RemoteAddr()
	log.Println("client connected:", remote)
	defer func() {
		conn.Close()
		if r := recover(); r != nil {
			log.Println("client", remote, "crashed the handler, recovered:", r)
			return
		}
		log.Println("client disconnected:", remote)
	}()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		args, err := readCommand(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Println("client", remote, "read error:", err)
			}
			return
		}
		if len(args) == 0 {
			continue
		}
		handleCommand(store, writer, args)
	}
}
