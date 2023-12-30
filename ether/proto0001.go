package ether

import "encoding/binary"

func (c *Connection) serve0001() {
	c.authState = 1
	
	if err := c.ack(); err != nil {
		c.abandon()
		return
	}

	var buff []byte
	pass := false

	for !pass {
		l, err := c.bind.Read(buff)

		if err != nil {
			c.log.Warn("There has been an error in pkt reading", err)
			c.handleErr(1, 0xff)

			continue
		}

		// Check minimal length
		if l < 4 {
			c.log.Warn("There has been an error while receiving the key", err)
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
		if uint16(len(buff)) != uint16(4) + keyLength {
			c.log.Warn("Key payload malformation")
			c.handleErr(1, 0xac)

			continue
		}

		// Store key and key length
		c.keyLength = keyLength 
		c.key = buff[4:4+keyLength]

		pass = true
	}

	c.authState = 2

	if err := c.ack(); err != nil {
		c.abandon() // there is no reason to reach this
		return
	}

	for {
		l, err := c.bind.Read(buff)

		if err != nil {
			c.log.Warn("There has been an error reading pkt", err)
			c.handleErr(2, 0xff)

			continue
		}

		// should at least have a cons
		if l < 1 {
			c.log.Warn("Packet malformed")
			c.handleErr(2, 0xba)
		}


		switch buff[0] {
		case 0xba:
			// Asking for a key
			c.handleErr(2, 0xfe)
			continue
		case 0xee:
			// Knock
			c.handleErr(2, 0xfe)
			continue
		case 0xab:
			// Knock ans
			c.log.Warn("Pckt out of path")
			c.handleErr(2, 0xe0)
			continue
		case 0xda:
			// Message
			c.handleErr(2, 0xfe)
			continue
		case 0xaf:
			// Room termination
			c.handleErr(2, 0xfe)
			continue
		case 0xbf:
			// Disconnect
			c.log.Info("Client disconnecting")
			c.ack()

			return

		default:
			c.log.Warn("Unknown packet:", buff[0])
			c.handleErr(2, 0xfd)
			continue
		}
	}
}

