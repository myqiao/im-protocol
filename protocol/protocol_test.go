package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
)

// TestFrameCreation tests creating a new Frame with different options
func TestFrameCreation(t *testing.T) {
	// Test basic frame creation
	body := []byte("test message")
	frame, err := NewFrame(FrameTypeJSON, body)
	if err != nil {
		t.Fatalf("Failed to create frame: %v", err)
	}

	if frame.Type != FrameTypeJSON {
		t.Errorf("Expected frame type %d, got %d", FrameTypeJSON, frame.Type)
	}

	if frame.Version != CurrentProtocolVersion {
		t.Errorf("Expected version %d, got %d", CurrentProtocolVersion, frame.Version)
	}

	if !bytes.Equal(frame.Body, body) {
		t.Errorf("Expected body %v, got %v", body, frame.Body)
	}

	if frame.GetBodyLength() != uint32(len(body)) {
		t.Errorf("Expected body length %d, got %d", len(body), frame.GetBodyLength())
	}

	// Test frame creation with custom version
	customVersion := uint8(2)
	frame, err = NewFrame(FrameTypeJSON, body, WithVersion(customVersion))
	if err != nil {
		t.Fatalf("Failed to create frame with custom version: %v", err)
	}

	if frame.Version != customVersion {
		t.Errorf("Expected version %d, got %d", customVersion, frame.Version)
	}

	// Test frame creation with zero copy
	frame, err = NewFrame(FrameTypeJSON, body, WithZeroCopy(true))
	if err != nil {
		t.Fatalf("Failed to create frame with zero copy: %v", err)
	}

	// Test invalid frame type
	_, err = NewFrame(99, body)
	if err == nil {
		t.Error("Expected error for invalid frame type, got nil")
	}

	if !IsFrameTypeError(err) {
		t.Error("Expected frame type error")
	}

	// Test unsupported version
	_, err = NewFrame(FrameTypeJSON, body, WithVersion(99))
	if err == nil {
		t.Error("Expected error for unsupported version, got nil")
	}

	if !IsVersionError(err) {
		t.Error("Expected version error")
	}
}

// TestFrameEncodeDecode tests encoding and decoding a Frame
func TestFrameEncodeDecode(t *testing.T) {
	body := []byte("test message")
	frame, err := NewFrame(FrameTypeJSON, body)
	if err != nil {
		t.Fatalf("Failed to create frame: %v", err)
	}

	// Test encoding
	data, err := frame.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame: %v", err)
	}

	// Verify header
	if len(data) < FrameHeaderLength {
		t.Errorf("Encoded data too short: %d bytes", len(data))
	}

	if data[0] != CurrentProtocolVersion {
		t.Errorf("Expected version %d, got %d", CurrentProtocolVersion, data[0])
	}

	if data[2] != FrameTypeJSON {
		t.Errorf("Expected frame type %d, got %d", FrameTypeJSON, data[2])
	}

	bodyLength := binary.BigEndian.Uint32(data[3:7])
	if bodyLength != uint32(len(body)) {
		t.Errorf("Expected body length %d, got %d", len(body), bodyLength)
	}

	// Test decoding
	decodedFrame, err := Decode(data)
	if err != nil {
		t.Fatalf("Failed to decode frame: %v", err)
	}

	if decodedFrame.Type != frame.Type {
		t.Errorf("Expected frame type %d, got %d", frame.Type, decodedFrame.Type)
	}

	if decodedFrame.Version != frame.Version {
		t.Errorf("Expected version %d, got %d", frame.Version, decodedFrame.Version)
	}

	if !bytes.Equal(decodedFrame.Body, frame.Body) {
		t.Errorf("Expected body %v, got %v", frame.Body, decodedFrame.Body)
	}
}

// TestFrameEncodeTo tests encoding to a writer
func TestFrameEncodeTo(t *testing.T) {
	body := []byte("test message")
	frame, err := NewFrame(FrameTypeJSON, body)
	if err != nil {
		t.Fatalf("Failed to create frame: %v", err)
	}

	var buf bytes.Buffer
	n, err := frame.EncodeTo(&buf)
	if err != nil {
		t.Fatalf("Failed to encode frame to writer: %v", err)
	}

	expectedLength := FrameHeaderLength + len(body)
	if n != expectedLength {
		t.Errorf("Expected to write %d bytes, wrote %d", expectedLength, n)
	}

	if buf.Len() != expectedLength {
		t.Errorf("Expected buffer length %d, got %d", expectedLength, buf.Len())
	}

	// Verify the data
	decodedFrame, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("Failed to decode frame: %v", err)
	}

	if !bytes.Equal(decodedFrame.Body, body) {
		t.Errorf("Expected body %v, got %v", body, decodedFrame.Body)
	}
}

// TestFrameEncodeToBytes tests encoding to a byte slice
func TestFrameEncodeToBytes(t *testing.T) {
	body := []byte("test message")
	frame, err := NewFrame(FrameTypeJSON, body)
	if err != nil {
		t.Fatalf("Failed to create frame: %v", err)
	}

	totalLength := FrameHeaderLength + len(body)
	buf := make([]byte, totalLength)

	n, err := frame.EncodeToBytes(buf)
	if err != nil {
		t.Fatalf("Failed to encode frame to bytes: %v", err)
	}

	if n != totalLength {
		t.Errorf("Expected to write %d bytes, wrote %d", totalLength, n)
	}

	// Verify the data
	decodedFrame, err := Decode(buf)
	if err != nil {
		t.Fatalf("Failed to decode frame: %v", err)
	}

	if !bytes.Equal(decodedFrame.Body, body) {
		t.Errorf("Expected body %v, got %v", body, decodedFrame.Body)
	}

	// Test with insufficient buffer size
	smallBuf := make([]byte, totalLength-1)
	_, err = frame.EncodeToBytes(smallBuf)
	if err == nil {
		t.Error("Expected error for insufficient buffer size, got nil")
	}
}

// TestSyncFrame tests the concurrent-safe SyncFrame
func TestSyncFrame(t *testing.T) {
	body := []byte("test message")
	syncFrame, err := NewSyncFrame(FrameTypeJSON, body)
	if err != nil {
		t.Fatalf("Failed to create sync frame: %v", err)
	}

	// Test basic operations
	if syncFrame.GetType() != FrameTypeJSON {
		t.Errorf("Expected frame type %d, got %d", FrameTypeJSON, syncFrame.GetType())
	}

	if !bytes.Equal(syncFrame.GetBody(), body) {
		t.Errorf("Expected body %v, got %v", body, syncFrame.GetBody())
	}

	// Test setting new body
	newBody := []byte("new test message")
	syncFrame.SetBody(newBody)

	if !bytes.Equal(syncFrame.GetBody(), newBody) {
		t.Errorf("Expected new body %v, got %v", newBody, syncFrame.GetBody())
	}

	if syncFrame.GetBodyLength() != uint32(len(newBody)) {
		t.Errorf("Expected body length %d, got %d", len(newBody), syncFrame.GetBodyLength())
	}

	// Test concurrent access
	var wg sync.WaitGroup
	iterations := 100

	// Concurrent writes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			newBody := []byte("message " + string(rune(id)))
			syncFrame.SetBody(newBody)
		}(i)
	}

	// Concurrent reads
	readResults := make([][]byte, 0, iterations)
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readBody := syncFrame.GetBody()
			readResults = append(readResults, readBody)
		}()
	}

	wg.Wait()

	// Verify all read results are valid (non-nil and reasonable length)
	for i, result := range readResults {
		if result == nil {
			t.Errorf("Read result %d is nil", i)
		}
		if len(result) == 0 {
			t.Errorf("Read result %d is empty", i)
		}
	}

	// Test WithLock
	syncFrame.WithLock(func(f *Frame) {
		f.Type = FrameTypeProtobuf
	})

	if syncFrame.GetType() != FrameTypeProtobuf {
		t.Errorf("Expected frame type %d after WithLock, got %d", FrameTypeProtobuf, syncFrame.GetType())
	}

	// Test Clone
	clone := syncFrame.Clone()
	if clone.GetType() != syncFrame.GetType() {
		t.Errorf("Clone type mismatch: expected %d, got %d", syncFrame.GetType(), clone.GetType())
	}

	if !bytes.Equal(clone.GetBody(), syncFrame.GetBody()) {
		t.Errorf("Clone body mismatch: expected %v, got %v", syncFrame.GetBody(), clone.GetBody())
	}

	// Verify it's a deep copy
	clone.SetBody([]byte("clone body"))
	if bytes.Equal(clone.GetBody(), syncFrame.GetBody()) {
		t.Error("Clone should be a deep copy, but bodies still match after modification")
	}
}

// TestStreamDecoder tests the StreamDecoder functionality
func TestStreamDecoder(t *testing.T) {
	// Create test frames
	frame1, err := NewFrame(FrameTypeJSON, []byte("message1"))
	if err != nil {
		t.Fatalf("Failed to create frame1: %v", err)
	}

	frame2, err := NewFrame(FrameTypeJSON, []byte("message2"))
	if err != nil {
		t.Fatalf("Failed to create frame2: %v", err)
	}

	// Encode frames
	data1, err := frame1.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame1: %v", err)
	}

	data2, err := frame2.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame2: %v", err)
	}

	// Create stream decoder
	decoder := NewStreamDecoder()

	// Test feeding incomplete data
	partialData := data1[:len(data1)/2]
	err = decoder.Feed(partialData)
	if err != nil {
		t.Fatalf("Failed to feed partial data: %v", err)
	}

	// Try to decode incomplete frame
	decodedFrame, err := decoder.TryDecode()
	if err != nil {
		t.Fatalf("Unexpected error decoding incomplete frame: %v", err)
	}

	if decodedFrame != nil {
		t.Error("Expected nil when decoding incomplete frame")
	}

	// Feed remaining data
	remainingData := data1[len(data1)/2:]
	err = decoder.Feed(remainingData)
	if err != nil {
		t.Fatalf("Failed to feed remaining data: %v", err)
	}

	// Decode complete frame
	decodedFrame, err = decoder.TryDecode()
	if err != nil {
		t.Fatalf("Failed to decode frame: %v", err)
	}

	if decodedFrame == nil {
		t.Error("Expected frame after feeding complete data")
	}

	if !bytes.Equal(decodedFrame.Body, frame1.Body) {
		t.Errorf("Expected body %v, got %v", frame1.Body, decodedFrame.Body)
	}

	// Test feeding multiple frames at once
	err = decoder.Feed(append(data1, data2...))
	if err != nil {
		t.Fatalf("Failed to feed combined data: %v", err)
	}

	// Decode first frame
	decodedFrame, err = decoder.TryDecode()
	if err != nil {
		t.Fatalf("Failed to decode first frame: %v", err)
	}

	if !bytes.Equal(decodedFrame.Body, frame1.Body) {
		t.Errorf("Expected body %v, got %v", frame1.Body, decodedFrame.Body)
	}

	// Decode second frame
	decodedFrame, err = decoder.TryDecode()
	if err != nil {
		t.Fatalf("Failed to decode second frame: %v", err)
	}

	if !bytes.Equal(decodedFrame.Body, frame2.Body) {
		t.Errorf("Expected body %v, got %v", frame2.Body, decodedFrame.Body)
	}

	// No more frames should be available
	decodedFrame, err = decoder.TryDecode()
	if err != nil {
		t.Fatalf("Unexpected error decoding nothing: %v", err)
	}

	if decodedFrame != nil {
		t.Error("Expected nil when no more frames")
	}
}

// TestStreamDecoderDecodeFromReader tests decoding from a reader
func TestStreamDecoderDecodeFromReader(t *testing.T) {
	// Create test frames
	frame1, err := NewFrame(FrameTypeJSON, []byte("message1"))
	if err != nil {
		t.Fatalf("Failed to create frame1: %v", err)
	}

	frame2, err := NewFrame(FrameTypeJSON, []byte("message2"))
	if err != nil {
		t.Fatalf("Failed to create frame2: %v", err)
	}

	// Encode frames
	data1, err := frame1.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame1: %v", err)
	}

	data2, err := frame2.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame2: %v", err)
	}

	// Create a reader with the combined data
	combinedData := append(data1, data2...)
	reader := bytes.NewReader(combinedData)

	// Create stream decoder
	decoder := NewStreamDecoder()

	// Test DecodeFromReader
	decodedFrame, err := decoder.DecodeFromReader(reader)
	if err != nil {
		t.Fatalf("Failed to decode from reader: %v", err)
	}

	if decodedFrame == nil {
		t.Error("Expected frame from reader")
	}

	if !bytes.Equal(decodedFrame.Body, frame1.Body) {
		t.Errorf("Expected body %v, got %v", frame1.Body, decodedFrame.Body)
	}

	// Decode second frame
	decodedFrame, err = decoder.DecodeFromReader(reader)
	if err != nil {
		t.Fatalf("Failed to decode second frame from reader: %v", err)
	}

	if !bytes.Equal(decodedFrame.Body, frame2.Body) {
		t.Errorf("Expected body %v, got %v", frame2.Body, decodedFrame.Body)
	}

	// No more data
	decodedFrame, err = decoder.DecodeFromReader(reader)
	if err != nil && err != io.EOF {
		t.Fatalf("Unexpected error decoding end of reader: %v", err)
	}

	if decodedFrame != nil {
		t.Error("Expected nil at end of reader")
	}
}

// TestStreamDecoderReset tests resetting the StreamDecoder
func TestStreamDecoderReset(t *testing.T) {
	frame, err := NewFrame(FrameTypeJSON, []byte("test message"))
	if err != nil {
		t.Fatalf("Failed to create frame: %v", err)
	}

	data, err := frame.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame: %v", err)
	}

	decoder := NewStreamDecoder()
	err = decoder.Feed(data[:len(data)/2])
	if err != nil {
		t.Fatalf("Failed to feed data: %v", err)
	}

	if decoder.Buffered() == 0 {
		t.Error("Expected buffered data after feed")
	}

	decoder.Reset()

	if decoder.Buffered() != 0 {
		t.Error("Expected no buffered data after reset")
	}
}

// TestErrorHandling tests various error scenarios
func TestErrorHandling(t *testing.T) {
	// Test message too long - should be caught during encoding
	largeBody := make([]byte, MaxMessageLength+1)
	for i := range largeBody {
		largeBody[i] = 'a'
	}

	// NewFrame might not check body length, but Encode should
	frame, err := NewFrame(FrameTypeJSON, largeBody, WithZeroCopy(true))
	if err != nil {
		t.Fatalf("Failed to create frame with large body: %v", err)
	}

	// Encode should fail with message too long error
	_, err = frame.Encode()
	if err == nil {
		t.Error("Expected error for message too long during encoding, got nil")
	}

	if !IsMessageTooLongError(err) {
		t.Error("Expected message too long error")
	}

	// Test encoding with invalid version
	frame, err = NewFrame(FrameTypeJSON, []byte("test"))
	if err != nil {
		t.Fatalf("Failed to create frame: %v", err)
	}

	frame.Version = uint8(99) // Set invalid version
	_, err = frame.Encode()
	if err == nil {
		t.Error("Expected error for invalid version during encode, got nil")
	}

	if !IsVersionError(err) {
		t.Error("Expected version error")
	}

	// Test decoding invalid frame
	invalidData := []byte("invalid frame data")
	_, err = Decode(invalidData)
	if err == nil {
		t.Error("Expected error for invalid frame, got nil")
	}

	// Test ProtocolError unwrapping
	originalErr := errors.New("original error")
	protocolErr := &ProtocolError{
		Code:     ErrCodeUnknown,
		Message:  "protocol error",
		Original: originalErr,
	}

	if !errors.Is(protocolErr, protocolErr) {
		t.Error("ProtocolError should be equal to itself")
	}

	if !errors.Is(protocolErr, originalErr) {
		t.Error("ProtocolError should wrap original error")
	}

	if errors.Unwrap(protocolErr) != originalErr {
		t.Error("ProtocolError unwrap should return original error")
	}
}

// TestStreamDecoderMaxBufferSize tests the max buffer size limit
func TestStreamDecoderMaxBufferSize(t *testing.T) {
	decoder := NewStreamDecoder(100) // Max buffer size 100 bytes

	largeData := make([]byte, 150)
	for i := range largeData {
		largeData[i] = 'a'
	}

	err := decoder.Feed(largeData)
	if err == nil {
		t.Error("Expected error for exceeding max buffer size, got nil")
	}

	if !IsMessageTooLongError(err) {
		t.Error("Expected message too long error")
	}
}
