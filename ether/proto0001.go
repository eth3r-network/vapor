//
// This file is part of the eTh3r project, written, hosted and distributed under MIT License
//  - eTh3r network, 2023-2024
//

package ether

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
)

func (c *Connection) serve0001(manager *Manager) {
	c.log.Info("ola from serve0001")

	c.authState = 1

	if err := c.ack(); checkErr(err) {
		c.log.Warn("Error sending ack", "err", err)
		c.abandon()
		return
	}

	buff := make([]byte, 1024)
	pass := false

	for !pass {
		l, err := c.bind.Read(buff)

		if checkErr(err) {
			c.log.Warn("There has been an error in pkt reading", "err", err)
			c.handleErr(1, 0xff)

			continue
		}

		// Check minimal length
		if l < 4 {
			c.log.Warn("There has been an error while receiving the key", "err", err)
			c.handleErr(1, 0xaa)

			continue
		}

		// Check pkt constant
		if buff[0] != 0x0e || buff[1] != 0x1f {
			c.log.Warn("Key packet malformed")
			c.handleErr(1, 0xab)

			continue
		}

		// Extract key length
		keyLength := binary.BigEndian.Uint16(buff[2:4])

		// Verify message length
		if uint16(l) != uint16(4)+keyLength {
			c.log.Warn("Key payload malformation", "len", l)
			c.handleErr(1, 0xac)

			continue
		}

		// Store key and key length
		c.keyLength = keyLength
		c.key = buff[4 : 4+keyLength]

		pass = true
	}

	c.authState = 2
	c.log.Info("Authstate 2, key passed", "length", c.keyLength)

	if err := c.ComputeKeyId(); err != 0 {
		c.log.Warn("Could not derive KeyId from PubKey", "err", err)
		c.handleErr(2, 0xad)
	}

	manager.RegisterConnection(c)

	c.authState = 3

	if err := c.ack(); checkErr(err) {
		c.abandon() // there is no reason to reach this
		return
	}

	for {
		l, err := c.bind.Read(buff)

		if errors.Is(err, io.EOF) {
			c.log.Warn("Client disconnected")
			return
		}

		if checkErr(err) {
			c.log.Warn("There has been an error reading pkt", "err", err)
			c.handleErr(3, 0xff)

			continue
		}

		// should at least have a cons
		if l < 1 {
			c.log.Warn("Packet malformed")
			c.handleErr(3, 0xba)
		}

		switch buff[0] {
		case 0xba:
			// Key retrieval
			c.log.Debug("Fetching user", "id", hex.EncodeToString(buff[1:]))
			conn := manager.FetchUserById(buff[1:]) // conn is c2's connection

			if conn == nil {
				c.log.Warn("Could not find user")

				respBuff := []byte{0xca}
				respBuff = append(respBuff[:], buff[:]...)

				_, err := c.bind.Write(respBuff)

				if err != nil {
					c.log.Warn("Could not send message")
				}

				continue
			}

			keyBuff := make([]byte, 2)                             // [length, key]
			binary.LittleEndian.PutUint16(keyBuff, conn.keyLength) // append the key length as uint16
			keyBuff = append(keyBuff, conn.key...)                 // append the key

			respBuff := []byte{0xa0, 0xba}
			respBuff = append(respBuff[:], keyBuff[:]...) // prepend the pck id

			_, err := c.bind.Write(respBuff) // write

			if err != nil {
				c.log.Warn("Could not send the user key, internal server error", "err", err)
			}

			continue
		case 0xee:
			// Knock
			c2 := buff[1:] // c2 is [length, keyId]

			kLength := uint(c2[0])

			if uint(l-2-1) != kLength { // 2 is 0xeeLL
				c.log.Warn("Wrong packet length", "l-3", l-3, "shouldBe", kLength)
				c.handleErr(2, 0xa1)

				continue
			}

			c.log.Debug("Fetching user", c2[1:])
			c2Conn := manager.FetchUserById(c2[1:])

			if c2Conn == nil {
				c.log.Warn("Could not find c2")
				c.handleErr(2, 0xad) // TODO: add 0xad as user not found

				continue
			}

			c2Conn.SendKnock0001(c) // send knock to c2 from c

			if _, err := c.bind.Write([]byte{0xa0, 0xee}); err != nil {
				c.log.Warn("Could not send the ack pkg", "err", err)

				continue
			}

			continue
		case 0xab:
			// Knock ans

			respVal := (buff[1] == 0x01)
			c.log.Debug("Knock ans", "res", respVal)

			c2 := buff[2:]

			kLength := uint(c2[0])

			if uint(l-3-2) != kLength { // 3 is 0xabRR[LL,...]
				c.log.Warn("Wrong packet length", "l-5", l-5, "kLength", kLength)
				c.handleErr(2, 0xa1)

				continue
			}

			c.log.Debug("Fetching user", "uid", hex.EncodeToString(c2[1:]))
			c2Conn := manager.FetchUserById(c2[1:])

			if c2Conn == nil {
				c.log.Warn("Could not find c2")
				c.handleErr(2, 0xad) // TODO: add 0xad as user not found

				continue
			}

			if _, err := c2Conn.bind.Write(buff); err != nil {
				c2Conn.log.Warn("Could not send pkg")
			}

			r := manager.SpawnRoom()

			c.NotifyRoom(r, c2Conn)
			c2Conn.NotifyRoom(r, c)

			continue
		case 0xda:
			// Message
			var ridLength uint8 = buff[1]
			rid := buff[2 : 2+ridLength]

			room := manager.FetchRoom(rid)
			room.SendMessageToRecipients0001(buff, c)

			if _, err := c.bind.Write([]byte{0xa0, 0xda}); err != nil {
				c.log.Warn("Could not send ack to message")
			}

			continue
		case 0xaf:
			// Room termination

			var ridLength uint8 = buff[1]
			rid := buff[2 : 2+ridLength]

			room := manager.FetchRoom(rid)
			room.SendMessageToAllRecipients0001(buff)

			manager.DropRoom(room)

			continue
		case 0xbf:
			// Disconnect
			c.log.Info("Client disconnecting")
			_ = c.ack()

			return

		default:
			c.log.Warn("Unknown packet:", "pkid", buff[0])
			c.handleErr(2, 0xfd)
			continue
		}
	}
}

func (c2 *Connection) SendKnock0001(c *Connection) error {
	// Send a knock to c2 from c1
	buff := []byte{0xae}

	buff = append(buff, byte(c.keyIdLength))
	buff = append(buff, c.keyId...)

	_, err := c2.bind.Write(buff)

	return err
}

func (c *Connection) NotifyRoom(r *Room, c2 *Connection) error {
	// Notify the creation of the room R with the user c2 in it

	buff := []byte{0xac}

	buff = append(buff, byte(r.roomIdLength))
	buff = append(buff, r.roomId...)

	buff = append(buff, byte(c2.keyIdLength))
	buff = append(buff, c2.keyId...)

	_, err := c.bind.Write(buff)

	return err
}

func (r *Room) SendMessageToAllRecipients0001(buff []byte) []error {
	return r.SendMessageToRecipients0001(buff, nil)
}

func (r *Room) SendMessageToRecipients0001(buff []byte, sender *Connection) []error {
	var errs []error
	var err error

	for _, client := range r.clients {
		if sender == nil || compare(client.keyId, sender.keyId) {
			_, err = client.bind.Write(buff)
			errs = append(errs, err)
		}
	}

	return errs
}
