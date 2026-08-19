package main

import (
	"bufio"
	"context"
	"fmt"
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

	fmt.Println("mini-redis listening on", address)

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
				return err
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConnection(store, conn)
		}()
	}
}

func handleConnection(store *Store, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		args, err := readCommand(reader)
		if err != nil {
			return
		}
		if len(args) == 0 {
			continue
		}
		handleCommand(store, writer, args)
	}
}
