package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// socksHandshake reads a SOCKS5 (or SOCKS4/4a) CONNECT request and returns
// the destination "host:port" and whether the client spoke SOCKS4. Only
// no-auth is offered.
func socksHandshake(c net.Conn) (dest string, v4 bool, err error) {
	c.SetReadDeadline(time.Now().Add(15 * time.Second))
	defer c.SetReadDeadline(time.Time{})

	var ver [1]byte
	if _, err := io.ReadFull(c, ver[:]); err != nil {
		return "", false, err
	}
	switch ver[0] {
	case 5:
		dest, err = socks5(c)
		return dest, false, err
	case 4:
		dest, err = socks4(c)
		return dest, true, err
	}
	return "", false, fmt.Errorf("unsupported SOCKS version %d", ver[0])
}

func socks5(c net.Conn) (string, error) {
	var n [1]byte
	if _, err := io.ReadFull(c, n[:]); err != nil {
		return "", err
	}
	methods := make([]byte, n[0])
	if _, err := io.ReadFull(c, methods); err != nil {
		return "", err
	}
	noAuth := false
	for _, m := range methods {
		if m == 0 {
			noAuth = true
		}
	}
	if !noAuth {
		c.Write([]byte{5, 0xff})
		return "", errors.New("client requires authentication")
	}
	c.Write([]byte{5, 0})

	var hdr [4]byte // ver cmd rsv atyp
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return "", err
	}
	if hdr[1] != 1 { // CONNECT
		c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("unsupported command %d", hdr[1])
	}
	var host string
	switch hdr[3] {
	case 1:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(c, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()
	case 4:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(c, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()
	case 3:
		if _, err := io.ReadFull(c, n[:]); err != nil {
			return "", err
		}
		name := make([]byte, n[0])
		if _, err := io.ReadFull(c, name); err != nil {
			return "", err
		}
		host = string(name)
	default:
		return "", fmt.Errorf("bad address type %d", hdr[3])
	}
	var p [2]byte
	if _, err := io.ReadFull(c, p[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(p[:])))), nil
}

// socks4 handles SOCKS4 and 4a (some old tools still speak it).
func socks4(c net.Conn) (string, error) {
	var hdr [7]byte // cmd port(2) ip(4)
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return "", err
	}
	if hdr[0] != 1 {
		return "", fmt.Errorf("unsupported SOCKS4 command %d", hdr[0])
	}
	readStr := func() (string, error) {
		var b []byte
		var one [1]byte
		for {
			if _, err := io.ReadFull(c, one[:]); err != nil {
				return "", err
			}
			if one[0] == 0 {
				return string(b), nil
			}
			if len(b) > 255 {
				return "", errors.New("SOCKS4 field too long")
			}
			b = append(b, one[0])
		}
	}
	if _, err := readStr(); err != nil { // user id
		return "", err
	}
	port := int(binary.BigEndian.Uint16(hdr[1:3]))
	ip := net.IP(hdr[3:7])
	host := ip.String()
	if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] != 0 { // 4a: name follows
		name, err := readStr()
		if err != nil {
			return "", err
		}
		host = name
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// socksReply answers the client after trying to reach the destination.
func socksReply(c net.Conn, v4 bool, dialErr error) {
	if v4 {
		code := byte(0x5a)
		if dialErr != nil {
			code = 0x5b
		}
		c.Write([]byte{0, code, 0, 0, 0, 0, 0, 0})
		return
	}
	code := byte(0)
	if dialErr != nil {
		code = 5 // connection refused
	}
	c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}
