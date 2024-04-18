//
// This file is part of the eTh3r project, written, hosted and distributed under MIT License
//  - eTh3r network, 2023-2024
//

package ether

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strconv"
)

type Room struct {
	roomId       []byte
	roomIdLength uint
	clients      []*Connection
}

type Connection struct {
	authState   int
	key         []byte
	keyId       []byte
	keyLength   uint16
	keyIdLength uint
	rooms       []*Room

	bind net.Conn
	log  *slog.Logger
}

func InitialiseConnection(conn net.Conn, log *slog.Logger) *Connection {
	newConn := new(Connection)

	newConn.authState = 0
	newConn.bind = conn
	newConn.log = log

	return newConn
}

func (c *Connection) handleErr(level int, _type byte) {
	_, err := c.bind.Write([]byte{_type})

	if err != nil {
		c.log.Warn("There has been an error transmitting the error code. Bad luck .-.", "level", level)
	}
}

func (c *Connection) ack() error {
	_, err := c.bind.Write([]byte{0xa0})

	if err != nil {
		c.log.Warn("There has been an error while sending ack packet to client:", err)
	}

	return err
}

func (c *Connection) abandon() {
	_ = c.bind.Close()
}

func (c *Connection) Serve(m *Manager) {
	buff := make([]byte, 1024)

	l, err := c.bind.Read(buff)

	if err != nil {
		c.log.Warn("An error has been caught reading a message")
		c.handleErr(0, 0xff)

		return
	}

	if l != 6 {
		c.log.Warn("The client sent a wrong amount of data: l=" + strconv.Itoa(l) + " " + hex.EncodeToString(buff))
		c.handleErr(0, 0xa1)

		return
	}

	if buff[0] != 0x05 || buff[1] != 0x31 || buff[2] != 0xb0 || buff[3] != 0x0b {
		c.log.Warn("Wrong payload, first message")
		c.log.Warn(hex.EncodeToString(buff[0:4]))
		c.handleErr(0, 0xa2)

		return
	}

	version := binary.BigEndian.Uint16(buff[4:6])

	switch version {
	case 0x0001:
		c.log.Info("Serving with version 0x0001")
		c.serve0001(m)
	default:
		c.log.Warn("Unsupported version", "ver", version)
		c.handleErr(0, 0xa4)
	}
}

func (c *Connection) NotifyRoomClose(r *Room) error {
	buff := []byte{0xaf}

	buff = append(buff, byte(r.roomIdLength))
	buff = append(buff, r.roomId...)

	_, err := c.bind.Write(buff)

	return err
}

func (c *Connection) ComputeKeyId() int {
	return 0
}

func checkErr(err error) bool {
	// handles EOF errors as nil, returns false if no error, true if should break

	if err == nil {
		return false
	}

	return true
}

func Test() {
	fmt.Println("Ola from protocol")
}
