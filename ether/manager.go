//
// This file is part of the eTh3r project, written, hosted and distributed under MIT License
//  - eTh3r network, 2023-2024
//

package ether

import (
	b64 "encoding/base64"
	"encoding/binary"
	"log/slog"
	"net"
	"strconv"
)

type Manager struct {
	listenPort    int
	stop          bool
	logger        *slog.Logger
	clients       []*Connection
	authedClients map[string]*Connection
	rooms         map[string]*Room
	rCnt          uint64
}

func Initialise(port int, log *slog.Logger) *Manager {
	newManager := new(Manager)

	newManager.listenPort = port
	newManager.stop = false
	newManager.logger = log

	newManager.authedClients = make(map[string]*Connection)
	newManager.rooms = make(map[string]*Room)
	newManager.rCnt = 0

	return newManager
}

func (m *Manager) Listen() error {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(m.listenPort))

	if err != nil {
		m.logger.Warn("An error got caught while trying to bind:", err)
		return err
	}

	for !m.stop {
		conn, err := ln.Accept()

		if err != nil {
			m.logger.Warn("There has been an issue when a client tried to connect:", err)
		}

		m.logger.Info("Serving", "addr", conn.RemoteAddr().String())

		client := InitialiseConnection(conn, m.logger)
		m.clients = append(m.clients, client)

		go client.Serve(m)
	}

	return nil
}

func (m *Manager) RegisterConnection(c *Connection) {
	hash := b64.StdEncoding.EncodeToString(c.keyId)

	m.authedClients[hash] = c
}

func (m *Manager) DropClient(c *Connection) {
	hash := b64.StdEncoding.EncodeToString(c.keyId)

	delete(m.authedClients, hash)
}

func (m *Manager) RegisterRoom(r *Room) {
	hash := b64.StdEncoding.EncodeToString(r.roomId)

	m.rooms[hash] = r
}

func (m *Manager) DropRoom(r *Room) {
	for _, client := range r.clients {
		client.NotifyRoomClose(r)
	}

	hash := b64.StdEncoding.EncodeToString(r.roomId)

	delete(m.rooms, hash)
}

func (m *Manager) FetchUserById(keyId []byte) *Connection {
	keyIdLength := uint(len(keyId))

	for _, conn := range m.clients {
		if conn.authState >= 3 && conn.keyIdLength == keyIdLength {
			// If the key has been handed over, keyid computed then, looking for a match
			if compare(keyId, conn.keyId) {
				return conn
			}
		}
	}

	return nil
}

func (m *Manager) SpawnRoom() *Room {
	nR := new(Room)

	binary.LittleEndian.PutUint64(nR.roomId, m.rCnt)
	nR.roomIdLength = uint(8)

	m.RegisterRoom(nR)

	return nR
}

func (m *Manager) FetchRoom(rid []byte) *Room {
	for _, r := range m.rooms {
		if compare(r.roomId, rid) {
			return r
		}
	}

	return nil
}

func compare(k1 []byte, k2 []byte) bool {
	if len(k1) != len(k2) {
		return false
	}

	for i, a := range k1 {
		if a != k2[i] {
			return false
		}
	}

	return true
}
