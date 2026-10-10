package uvox

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	delta = 0x9E3779B9
	mask  = 0xFFFFFFFF

	MsgAuth       uint16 = 0x1001
	MsgSetup      uint16 = 0x1002
	MsgBuffer     uint16 = 0x1003
	MsgStandby    uint16 = 0x1004
	MsgTerminate  uint16 = 0x1005
	MsgMaxPayload uint16 = 0x1008
	MsgCipher     uint16 = 0x1009
	MsgMIME       uint16 = 0x1040
	MsgIcyName    uint16 = 0x1100
	MsgIcyGenre   uint16 = 0x1101
	MsgIcyPub     uint16 = 0x1103
	MsgMP3Data    uint16 = 0x7000
)

// UvoxError represents a protocol or connection error suitable for display.
type UvoxError struct {
	Message string
}

func (e *UvoxError) Error() string {
	return e.Message
}

func xteaBlock(v0, v1 uint32, k [4]uint32) (uint32, uint32) {
	var s uint32 = 0
	for i := 0; i < 32; i++ {
		v0 = (v0 + ((((v1 << 4) ^ (v1 >> 5)) + v1) ^ (s + k[s&3]))) & mask
		s = (s + delta) & mask
		v1 = (v1 + ((((v0 << 4) ^ (v0 >> 5)) + v0) ^ (s + k[(s>>11)&3]))) & mask
	}
	return v0, v1
}

// XTEAHex encrypts data using XTEA with a 128-bit key and returns a hexadecimal string.
func XTEAHex(data, key []byte) string {
	// Pad or truncate key to 16 bytes
	keyPadded := make([]byte, 16)
	copy(keyPadded, key)

	var k [4]uint32
	k[0] = binary.BigEndian.Uint32(keyPadded[0:4])
	k[1] = binary.BigEndian.Uint32(keyPadded[4:8])
	k[2] = binary.BigEndian.Uint32(keyPadded[8:12])
	k[3] = binary.BigEndian.Uint32(keyPadded[12:16])

	// Pad data to multiple of 8 bytes with zeros
	padLen := len(data)
	if padLen%8 != 0 {
		padLen += 8 - (padLen % 8)
	}
	dataPadded := make([]byte, padLen)
	copy(dataPadded, data)

	var out strings.Builder
	for i := 0; i < len(dataPadded); i += 8 {
		v0 := binary.BigEndian.Uint32(dataPadded[i : i+4])
		v1 := binary.BigEndian.Uint32(dataPadded[i+4 : i+8])
		v0, v1 = xteaBlock(v0, v1, k)
		out.WriteString(fmt.Sprintf("%08x%08x", v0, v1))
	}
	return out.String()
}

// Config contains the connection settings for SHOUTcast v2.
type Config struct {
	Host     string
	Port     int
	SID      int
	User     string
	Password string
	Bitrate  int
	Name     string
	Genre    string
}

// UvoxSource manages the broadcast connection to a SHOUTcast DNAS v2 server.
type UvoxSource struct {
	cfg        Config
	conn       net.Conn
	maxPayload int
	mu         sync.Mutex
	closed     bool
}

// NewUvoxSource creates a new UvoxSource instance.
func NewUvoxSource(cfg Config) *UvoxSource {
	if cfg.Bitrate <= 0 {
		cfg.Bitrate = 128
	}
	if cfg.Name == "" {
		cfg.Name = "PyStreamer"
	}
	if cfg.Genre == "" {
		cfg.Genre = "Variado"
	}
	return &UvoxSource{
		cfg:        cfg,
		maxPayload: 16377,
	}
}

func (s *UvoxSource) friendlyError(reason, step string) error {
	if strings.Contains(reason, "Stream In Use") {
		return &UvoxError{Message: "Stream ocupado: detén el AutoDJ en tu panel"}
	}
	if strings.Contains(reason, "Deny") {
		return &UvoxError{Message: "Usuario, contraseña o Stream ID incorrectos"}
	}
	if strings.Contains(reason, "Stream ID Error") {
		return &UvoxError{Message: "Stream ID inválido"}
	}
	return &UvoxError{Message: fmt.Sprintf("%s: %s", step, reason)}
}

func (s *UvoxSource) send(msgType uint16, payload []byte) error {
	frame := make([]byte, 6+len(payload)+1)
	frame[0] = 0x5A
	frame[1] = 0x00
	binary.BigEndian.PutUint16(frame[2:4], msgType)
	binary.BigEndian.PutUint16(frame[4:6], uint16(len(payload)))
	copy(frame[6:], payload)
	frame[len(frame)-1] = 0x00

	_, err := s.conn.Write(frame)
	return err
}

func (s *UvoxSource) recv() (uint16, []byte, error) {
	hdr := make([]byte, 6)
	if _, err := io.ReadFull(s.conn, hdr); err != nil {
		return 0, nil, &UvoxError{Message: "El servidor cerró la conexión"}
	}
	if hdr[0] != 0x5A {
		return 0, nil, &UvoxError{Message: "Respuesta no válida del servidor"}
	}

	msgType := binary.BigEndian.Uint16(hdr[2:4])
	ln := int(binary.BigEndian.Uint16(hdr[4:6]))

	payload := make([]byte, ln+1)
	if _, err := io.ReadFull(s.conn, payload); err != nil {
		return 0, nil, &UvoxError{Message: "El servidor cerró la conexión"}
	}

	content := payload[:ln]
	content = bytes.TrimRight(content, "\x00")
	return msgType, content, nil
}

func (s *UvoxSource) request(msgType uint16, payload, step string) ([]byte, error) {
	pBytes := append([]byte(payload), 0x00)
	if err := s.send(msgType, pBytes); err != nil {
		return nil, &UvoxError{Message: fmt.Sprintf("Error enviando %s: %v", step, err)}
	}

	_, resp, err := s.recv()
	if err != nil {
		return nil, err
	}

	if !bytes.HasPrefix(resp, []byte("ACK")) {
		return nil, s.friendlyError(string(resp), step)
	}

	return resp, nil
}

// Connect establishes the TCP connection and completes the Ultravox 2.1 handshake.
func (s *UvoxSource) Connect(timeout time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return &UvoxError{Message: fmt.Sprintf("No se pudo conectar: %v", err)}
	}
	s.conn = conn
	s.closed = false

	handshakeErr := func() error {
		_ = s.conn.SetDeadline(time.Now().Add(timeout))

		// Step 1: Cipher request
		resp, err := s.request(MsgCipher, "2.1", "Cifra")
		if err != nil {
			return err
		}

		var key []byte
		idx := bytes.IndexByte(resp, ':')
		if idx != -1 && idx+1 < len(resp) {
			key = resp[idx+1:]
		}

		uid := XTEAHex([]byte(s.cfg.User), key)
		blob := XTEAHex([]byte(s.cfg.Password), key)

		// Step 2: Auth
		authPayload := fmt.Sprintf("2.1:%d:%s:%s", s.cfg.SID, uid, blob)
		if _, err := s.request(MsgAuth, authPayload, "Autenticación"); err != nil {
			return err
		}

		// Step 3: MIME
		if _, err := s.request(MsgMIME, "audio/mpeg", "Formato"); err != nil {
			return err
		}

		// Step 4: Setup bitrate
		bitratePayload := fmt.Sprintf("%d:%d", s.cfg.Bitrate, s.cfg.Bitrate)
		if _, err := s.request(MsgSetup, bitratePayload, "Bitrate"); err != nil {
			return err
		}

		// Step 5: Buffer
		if _, err := s.request(MsgBuffer, "32:16", "Buffer"); err != nil {
			return err
		}

		// Step 6: Max payload
		resp, err = s.request(MsgMaxPayload, "16377:1024", "Tamaño de mensaje")
		if err == nil {
			parts := strings.Split(string(resp), ":")
			if len(parts) >= 2 {
				if parsed, parseErr := strconv.Atoi(parts[1]); parseErr == nil && parsed > 0 {
					s.maxPayload = parsed
				}
			}
		}

		// Step 7: ICY metadata
		if _, err := s.request(MsgIcyName, s.cfg.Name, "Nombre"); err != nil {
			return err
		}
		if _, err := s.request(MsgIcyGenre, s.cfg.Genre, "Género"); err != nil {
			return err
		}
		if _, err := s.request(MsgIcyPub, "0", "Publicación"); err != nil {
			return err
		}

		// Step 8: Standby
		if err := s.send(MsgStandby, nil); err != nil {
			return &UvoxError{Message: fmt.Sprintf("Error enviando Standby: %v", err)}
		}

		_, standbyResp, err := s.recv()
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(standbyResp, []byte("ACK")) {
			return s.friendlyError(string(standbyResp), "Standby")
		}

		// Clear deadline for streaming
		_ = s.conn.SetDeadline(time.Time{})
		return nil
	}()

	if handshakeErr != nil {
		s.closeLocked(false)
		return handshakeErr
	}

	return nil
}

// SendAudio sends a buffer of MP3 encoded data split into frames.
func (s *UvoxSource) SendAudio(mp3Data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil || s.closed {
		return &UvoxError{Message: "Conexión cerrada"}
	}

	for i := 0; i < len(mp3Data); i += s.maxPayload {
		end := i + s.maxPayload
		if end > len(mp3Data) {
			end = len(mp3Data)
		}
		chunk := mp3Data[i:end]
		if err := s.send(MsgMP3Data, chunk); err != nil {
			return &UvoxError{Message: fmt.Sprintf("Error enviando audio: %v", err)}
		}
	}
	return nil
}

// Close gracefully terminates the stream and closes the socket.
func (s *UvoxSource) Close(terminate bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked(terminate)
}

func (s *UvoxSource) closeLocked(terminate bool) {
	if s.closed || s.conn == nil {
		return
	}
	s.closed = true
	if terminate {
		_ = s.send(MsgTerminate, nil)
	}
	_ = s.conn.Close()
	s.conn = nil
}
