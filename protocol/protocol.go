package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

// 常量定义
const (
	// FrameHeaderLength 帧头长度
	FrameHeaderLength = 7 // 版本号(1字节) + 子版本号(1字节) + 消息类型(1字节) + 消息体长度(4字节，大端序)       |
	// MaxMessageLength 最大消息长度
	MaxMessageLength = 1024 * 1024 // 1MB
	// FrameTypeJSON JSON格式
	FrameTypeJSON = 1
	// FrameTypeProtobuf Protobuf格式
	FrameTypeProtobuf = 2
	// FrameTypeMsgPack MsgPack格式
	FrameTypeMsgPack = 3

	// 版本常量
	ProtocolVersionV1      uint8 = 1                 // 初始版本（当前实现）
	ProtocolVersionV2      uint8 = 2                 // 迭代版本（比如新增字段/调整格式）
	CurrentProtocolVersion uint8 = ProtocolVersionV1 // 当前默认版本

	// 缓冲区大小常量
	smallBufferSize  = 2 * 1024 // 小缓冲区大小：2KB
	mediumBufferSize = 4 * 1024 // 中缓冲区大小：4KB
	largeBufferSize  = 8 * 1024 // 大缓冲区大小：8KB
)

// bufferPool 用于缓存不同规格的缓冲区，减少内存分配
// 分级缓冲区池，按大小分级缓存不同规格的缓冲区
type tieredBufferPool struct {
	smallPool  sync.Pool // 小缓冲区池 (2KB)
	mediumPool sync.Pool // 中缓冲区池 (4KB)
	largePool  sync.Pool // 大缓冲区池 (8KB)
}

// 全局分级缓冲区池实例
var bufferPool = &tieredBufferPool{
	smallPool: sync.Pool{
		New: func() interface{} {
			buf := make([]byte, smallBufferSize)
			return &buf
		},
	},
	mediumPool: sync.Pool{
		New: func() interface{} {
			buf := make([]byte, mediumBufferSize)
			return &buf
		},
	},
	largePool: sync.Pool{
		New: func() interface{} {
			buf := make([]byte, largeBufferSize)
			return &buf
		},
	},
}

// Get 根据所需大小从合适的池中获取缓冲区
func (p *tieredBufferPool) Get(size int) *[]byte {
	switch {
	case size <= smallBufferSize:
		return p.smallPool.Get().(*[]byte)
	case size <= mediumBufferSize:
		return p.mediumPool.Get().(*[]byte)
	case size <= largeBufferSize:
		return p.largePool.Get().(*[]byte)
	default:
		// 超大缓冲区直接创建，不缓存
		buf := make([]byte, size)
		return &buf
	}
}

// Put 将缓冲区放回合适的池中
func (p *tieredBufferPool) Put(bufPtr *[]byte) {
	if bufPtr == nil {
		return
	}

	buf := *bufPtr
	capacity := cap(buf)

	// 重置缓冲区长度为容量，以便下次使用
	*bufPtr = (*bufPtr)[:capacity]

	switch {
	case capacity == smallBufferSize:
		p.smallPool.Put(bufPtr)
	case capacity == mediumBufferSize:
		p.mediumPool.Put(bufPtr)
	case capacity == largeBufferSize:
		p.largePool.Put(bufPtr)
		// 其他大小的缓冲区不回收，让GC处理
	}
}

// SupportedVersions 支持的协议版本列表
var SupportedVersions = []uint8{ProtocolVersionV1, ProtocolVersionV2}

// ErrorCode 错误码类型
type ErrorCode uint8

// 错误码定义
const (
	// ErrCodeUnknown 未知错误
	ErrCodeUnknown ErrorCode = 0
	// ErrCodeMessageTooLong 消息过长
	ErrCodeMessageTooLong ErrorCode = 1
	// ErrCodeInvalidFrame 无效的帧格式
	ErrCodeInvalidFrame ErrorCode = 2
	// ErrCodeUnsupportedVersion 不支持的协议版本
	ErrCodeUnsupportedVersion ErrorCode = 3
	// ErrCodeInvalidFrameType 无效的帧类型
	ErrCodeInvalidFrameType ErrorCode = 4
	// ErrCodeBufferTooSmall 缓冲区太小
	ErrCodeBufferTooSmall ErrorCode = 5
)

// ProtocolError 自定义协议错误类型
type ProtocolError struct {
	// Code 错误码
	Code ErrorCode
	// Message 错误消息
	Message string
	// Original 原始错误（可选）
	Original error
}

// Error 实现error接口
func (e *ProtocolError) Error() string {
	return e.Message
}

// Unwrap 实现errors.Unwrap接口，支持错误链
func (e *ProtocolError) Unwrap() error {
	return e.Original
}

// Is 实现errors.Is接口，支持errors.Is判断
func (e *ProtocolError) Is(target error) bool {
	if t, ok := target.(*ProtocolError); ok {
		return e.Code == t.Code
	}
	return false
}

// 预定义错误实例
var (
	// ErrMessageTooLong 消息过长
	ErrMessageTooLong = &ProtocolError{Code: ErrCodeMessageTooLong, Message: "message too long"}
	// ErrInvalidFrame 无效的帧格式
	ErrInvalidFrame = &ProtocolError{Code: ErrCodeInvalidFrame, Message: "invalid frame format"}
	// ErrUnsupportedVersion 不支持的协议版本
	ErrUnsupportedVersion = &ProtocolError{Code: ErrCodeUnsupportedVersion, Message: "unsupported protocol version"}
	// ErrInvalidFrameType 无效的帧类型
	ErrInvalidFrameType = &ProtocolError{Code: ErrCodeInvalidFrameType, Message: "invalid frame type"}
	// ErrBufferTooSmall 缓冲区太小
	ErrBufferTooSmall = &ProtocolError{Code: ErrCodeBufferTooSmall, Message: "buffer too small"}
)

// NewMessageTooLongError 创建消息过长错误，包含实际长度和最大长度信息
func NewMessageTooLongError(actualLen, maxLen int) error {
	return &ProtocolError{
		Code:    ErrCodeMessageTooLong,
		Message: fmt.Sprintf("message too long: %d bytes, maximum allowed is %d bytes", actualLen, maxLen),
	}
}

// NewInvalidFrameError 创建无效帧错误，包含详细信息
func NewInvalidFrameError(detail string) error {
	return &ProtocolError{
		Code:    ErrCodeInvalidFrame,
		Message: fmt.Sprintf("invalid frame format: %s", detail),
	}
}

// NewUnsupportedVersionError 创建不支持的版本错误，包含实际版本和支持的版本列表
func NewUnsupportedVersionError(actualVersion uint8, supportedVersions []uint8) error {
	return &ProtocolError{
		Code:    ErrCodeUnsupportedVersion,
		Message: fmt.Sprintf("unsupported protocol version: %d, supported versions: %v", actualVersion, supportedVersions),
	}
}

// NewInvalidFrameTypeError 创建无效帧类型错误，包含实际类型和支持的类型列表
func NewInvalidFrameTypeError(actualType uint8, supportedTypes []uint8) error {
	return &ProtocolError{
		Code:    ErrCodeInvalidFrameType,
		Message: fmt.Sprintf("invalid frame type: %d, supported types: %v", actualType, supportedTypes),
	}
}

// IsFrameTypeError 检查错误是否为无效帧类型错误
func IsFrameTypeError(err error) bool {
	var pErr *ProtocolError
	return errors.As(err, &pErr) && pErr.Code == ErrCodeInvalidFrameType
}

// IsVersionError 检查错误是否为版本相关错误
func IsVersionError(err error) bool {
	var pErr *ProtocolError
	return errors.As(err, &pErr) && pErr.Code == ErrCodeUnsupportedVersion
}

// IsMessageTooLongError 检查错误是否为消息过长错误
func IsMessageTooLongError(err error) bool {
	var pErr *ProtocolError
	return errors.As(err, &pErr) && pErr.Code == ErrCodeMessageTooLong
}

// IsInvalidFrameError 检查错误是否为无效帧错误
func IsInvalidFrameError(err error) bool {
	var pErr *ProtocolError
	return errors.As(err, &pErr) && pErr.Code == ErrCodeInvalidFrame
}

// GetErrorCode 从错误中提取错误码
func GetErrorCode(err error) ErrorCode {
	var pErr *ProtocolError
	if errors.As(err, &pErr) {
		return pErr.Code
	}
	return ErrCodeUnknown
}

// Frame 协议帧结构
// +--------+--------+--------+--------+--------+--------+--------+--------+--------+
// | 版本号 (1字节) | 子版本号 (1字节) | 消息类型 (1字节) |       消息体长度 (4字节，大端序)       |
// +--------+--------+--------+--------+--------+--------+--------+--------+--------+
// |                              消息体 (可变长度)                              |
// +--------+--------+--------+--------+--------+--------+--------+--------+--------+
//
// 并发安全说明：
// Frame 实例非并发安全，多协程操作需加锁
type Frame struct {
	// Version 版本号
	Version uint8
	// SubVersion 子版本号，用于区分同一主版本下的不同格式
	SubVersion uint8
	// Type 消息类型
	Type uint8
	// bodyLength 消息体长度（私有字段，禁止外部直接修改）
	bodyLength uint32
	// Body 消息体
	Body []byte
}

// GetBodyLength 获取消息体长度
func (f *Frame) GetBodyLength() uint32 {
	return f.bodyLength
}

// GetSubVersion 获取子版本号
func (f *Frame) GetSubVersion() uint8 {
	return f.SubVersion
}

// SetSubVersion 设置子版本号
func (f *Frame) SetSubVersion(subVersion uint8) {
	f.SubVersion = subVersion
}

// SyncFrame 并发安全的协议帧结构
// 嵌入Frame并添加读写锁，支持多协程并发访问
//
// 使用场景：
//   - 当多个协程需要同时访问和修改同一个Frame实例时
//   - 网络编程中，一个连接的读写协程可能同时操作协议帧
//
// 实现中的重要细节：
//   - 嵌入Frame结构体，继承其所有字段和方法
//   - 添加sync.RWMutex读写锁，实现并发安全
//   - 提供WithLock方法，方便自定义并发安全操作
//   - 提供并发安全的Encode和Decode方法包装
//   - 提供SetBody和GetBody等并发安全的字段访问方法
//
// 使用示例：
//
//	syncFrame := NewSyncFrame(FrameTypeJSON, []byte("test"))
//	// 并发安全地设置body
//	syncFrame.SetBody([]byte("new content"))
//	// 并发安全地编码
//	data, err := syncFrame.Encode()
//	// 自定义并发安全操作
//	syncFrame.WithLock(func(f *Frame) {
//	    f.Type = FrameTypeProtobuf
//	})
type SyncFrame struct {
	// Frame 嵌入的协议帧
	Frame
	// mu 读写锁
	mu sync.RWMutex
}

// NewSyncFrame 创建新的并发安全协议帧
//
// 参数与NewFrame相同
func NewSyncFrame(frameType uint8, body []byte, options ...ConstructorOption) (*SyncFrame, error) {
	frame, err := NewFrame(frameType, body, options...)
	if err != nil {
		return nil, err
	}
	return &SyncFrame{Frame: *frame}, nil
}

// Encode 并发安全的编码方法
// 优化：减少临界区范围，提高并发性能
func (sf *SyncFrame) Encode() ([]byte, error) {
	// 第一步：读锁下读取必要字段
	sf.mu.RLock()

	// 复制必要字段到局部变量
	version := sf.Version
	subVersion := sf.SubVersion
	frameType := sf.Type
	body := make([]byte, len(sf.Body))
	copy(body, sf.Body)
	bodyLength := sf.bodyLength

	// 释放读锁，减少临界区范围
	sf.mu.RUnlock()

	// 验证消息体长度是否超过限制
	if len(body) > MaxMessageLength {
		return nil, NewMessageTooLongError(len(body), MaxMessageLength)
	}

	// 验证版本是否为支持的版本
	if !isSupportedVersion(version) {
		return nil, NewUnsupportedVersionError(version, SupportedVersions)
	}

	// 第二步：无锁编码
	// 计算总长度：帧头长度 + 消息体长度
	totalLength := FrameHeaderLength + len(body)

	// 从分级池中获取合适大小的缓冲区
	bufPtr := bufferPool.Get(totalLength)
	buf := *bufPtr

	// 如果是超大缓冲区（超过8KB），直接使用所需大小
	if cap(buf) < totalLength {
		buf = make([]byte, totalLength)
		*bufPtr = buf // 更新指针指向新缓冲区
	}

	// 重置缓冲区长度
	buf = buf[:totalLength]

	// 写入版本号
	buf[0] = version
	// 写入子版本号
	buf[1] = subVersion
	// 写入消息类型
	buf[2] = frameType
	// 写入消息体长度
	binary.BigEndian.PutUint32(buf[3:7], bodyLength)
	// 写入消息体
	copy(buf[7:], body)

	// 创建返回值副本，避免池中的缓冲区被修改
	result := make([]byte, totalLength)
	copy(result, buf)

	// 将缓冲区放回合适的池中
	bufferPool.Put(bufPtr)

	return result, nil
}

// Decode 并发安全的解码方法
// 注意：这个方法不会修改当前SyncFrame实例，而是返回一个新的SyncFrame实例
func (sf *SyncFrame) Decode(data []byte) (*SyncFrame, error) {
	frame, err := Decode(data)
	if err != nil {
		return nil, err
	}
	return &SyncFrame{Frame: *frame}, nil
}

// WithLock 提供自定义并发安全操作的方法
//
// 参数：
//
//	f - 回调函数，在锁保护下访问Frame实例
func (sf *SyncFrame) WithLock(f func(*Frame)) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	f(&sf.Frame)
}

// WithRLock 提供自定义并发安全读操作的方法
//
// 参数：
//
//	f - 回调函数，在读锁保护下访问Frame实例
func (sf *SyncFrame) WithRLock(f func(*Frame)) {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	f(&sf.Frame)
}

// SetBody 并发安全地设置消息体
func (sf *SyncFrame) SetBody(body []byte) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.Body = body
	sf.bodyLength = uint32(len(body))
}

// GetBody 并发安全地获取消息体（深拷贝，避免外部修改）
func (sf *SyncFrame) GetBody() []byte {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	body := make([]byte, len(sf.Body))
	copy(body, sf.Body)
	return body
}

// SetType 并发安全地设置消息类型
func (sf *SyncFrame) SetType(frameType uint8) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	if !isValidFrameType(frameType) {
		return NewInvalidFrameTypeError(frameType, []uint8{FrameTypeJSON, FrameTypeProtobuf, FrameTypeMsgPack})
	}
	sf.Type = frameType
	return nil
}

// GetType 并发安全地获取消息类型
func (sf *SyncFrame) GetType() uint8 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.Type
}

// SetVersion 并发安全地设置版本号
func (sf *SyncFrame) SetVersion(version uint8) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.Version = version
}

// GetVersion 并发安全地获取版本号
func (sf *SyncFrame) GetVersion() uint8 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.Version
}

// GetSubVersion 并发安全地获取子版本号
func (sf *SyncFrame) GetSubVersion() uint8 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.SubVersion
}

// SetSubVersion 并发安全地设置子版本号
func (sf *SyncFrame) SetSubVersion(subVersion uint8) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.SubVersion = subVersion
}

// Clone 创建SyncFrame的深拷贝
// 返回一个新的SyncFrame实例，包含完全相同的字段值，但Body是深拷贝的
func (sf *SyncFrame) Clone() *SyncFrame {
	sf.mu.RLock()
	defer sf.mu.RUnlock()

	// 深拷贝Body
	body := make([]byte, len(sf.Body))
	copy(body, sf.Body)

	return &SyncFrame{
		Frame: Frame{
			Version:    sf.Version,
			SubVersion: sf.SubVersion,
			Type:       sf.Type,
			bodyLength: sf.bodyLength,
			Body:       body,
		},
	}
}

// isValidFrameType 检查帧类型是否合法
func isValidFrameType(frameType uint8) bool {
	return frameType == FrameTypeJSON || frameType == FrameTypeProtobuf || frameType == FrameTypeMsgPack
}

// isSupportedVersion 检查协议版本是否受支持
func isSupportedVersion(version uint8) bool {
	for _, v := range SupportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// FrameOption 基础帧选项接口
type FrameOption interface {
	// applyFrame 应用选项到Frame
	applyFrame(*Frame) error
}

// ConstructorOption 构造期选项接口
// 用于Frame构造阶段的选项配置
// 实现了FrameOption接口，可用于NewFrame函数

type ConstructorOption interface {
	FrameOption
	// 标记为构造期选项
	isConstructorOption()
}

// EncodeOption 编码期选项接口
// 用于Encode方法的选项配置
// 实现了FrameOption接口，可用于Encode相关方法
type EncodeOption interface {
	FrameOption
	// 标记为编码期选项
	isEncodeOption()
}

// 版本选项实现
type versionOption struct {
	version uint8
}

func (o *versionOption) applyFrame(f *Frame) error {
	f.Version = o.version
	return nil
}

func (o *versionOption) isConstructorOption() {}

// WithVersion 设置帧版本选项
// 构造期选项，用于NewFrame函数
func WithVersion(version uint8) ConstructorOption {
	return &versionOption{version: version}
}

// 拷贝选项实现
type copyBodyOption struct {
	copy bool
}

func (o *copyBodyOption) applyFrame(f *Frame) error {
	// 这个选项在NewFrame中特殊处理，这里不做任何事
	return nil
}

func (o *copyBodyOption) isConstructorOption() {}

// WithCopyBody 设置是否深拷贝body选项
// 构造期选项，用于NewFrame函数
// true: 深拷贝body（默认），安全但有性能开销
// false: 浅拷贝body（零拷贝），性能好但需保证外部body不可变
func WithCopyBody(copy bool) ConstructorOption {
	return &copyBodyOption{copy: copy}
}

// WithZeroCopy 设置是否使用零拷贝模式
// 构造期选项，用于NewFrame函数
// true: 浅拷贝body（零拷贝），性能好但需保证外部body不可变
// false: 深拷贝body（默认），安全但有性能开销
// 注意：使用零拷贝时，外部body必须只读且生命周期长于Frame
func WithZeroCopy(zeroCopy bool) ConstructorOption {
	return &copyBodyOption{copy: !zeroCopy}
}

// 子版本选项实现
type subVersionOption struct {
	subVersion uint8
}

func (o *subVersionOption) applyFrame(f *Frame) error {
	f.SubVersion = o.subVersion
	return nil
}

func (o *subVersionOption) isConstructorOption() {}

// WithSubVersion 设置帧子版本选项
// 构造期选项，用于NewFrame函数
func WithSubVersion(subVersion uint8) ConstructorOption {
	return &subVersionOption{subVersion: subVersion}
}

// NewFrame 创建新的协议帧
//
// 参数：
//
//	frameType - 帧类型，支持的类型包括FrameTypeJSON、FrameTypeProtobuf、FrameTypeMsgPack
//
//	body - 消息体内容
//
//	options - 可选参数，支持：
//	  - WithVersion(version uint8): 指定协议版本
//	  - WithCopyBody(copy bool): 是否对body进行深拷贝
//
// 返回值：
//
//	*Frame - 成功时返回新创建的Frame结构体指针
//
//	error - 失败时返回错误信息
//
// 使用用例：
//
//  1. 创建JSON类型的帧（使用默认版本，浅拷贝）
//     frame, err := NewFrame(FrameTypeJSON, []byte("{\"message\":\"hello\"}"))
//     返回值：&Frame{Version:1, Type:1, BodyLength:18, Body:[]byte("{\"message\":\"hello\"}")}, nil
//
//  2. 创建JSON类型的帧（深拷贝body）
//     frame, err := NewFrame(FrameTypeJSON, []byte("{\"message\":\"hello\"}"), WithCopyBody(true))
//     返回值：&Frame{Version:1, Type:1, BodyLength:18, Body:深拷贝的字节数组}, nil
//
//  3. 创建Protobuf类型的帧（指定版本）
//     frame, err := NewFrame(FrameTypeProtobuf, protobufData, WithVersion(ProtocolVersionV1))
//     返回值：&Frame{Version:1, Type:2, BodyLength:len(protobufData), Body:protobufData}, nil
//
//  4. 创建无效类型的帧
//     frame, err := NewFrame(99, []byte("invalid"))
//     返回值：nil, ErrInvalidFrameType
//
// 实现中的重要细节：
//
//   - 自动计算消息体的长度并设置到Frame结构体的BodyLength字段
//   - 支持浅拷贝和深拷贝两种模式，浅拷贝默认开启以提高性能
//   - 校验帧类型的合法性，确保只有支持的帧类型才能被创建
//   - 使用选项模式提供类型安全的可选参数
func NewFrame(frameType uint8, body []byte, options ...ConstructorOption) (*Frame, error) {
	// 校验帧类型的合法性
	if !isValidFrameType(frameType) {
		return nil, NewInvalidFrameTypeError(frameType, []uint8{FrameTypeJSON, FrameTypeProtobuf, FrameTypeMsgPack})
	}

	// 解析可选参数
	v := CurrentProtocolVersion
	subVersion := uint8(0) // 默认子版本号为0
	copyBody := true       // 默认深拷贝，安全优先

	for _, opt := range options {
		switch o := opt.(type) {
		case *versionOption:
			v = o.version
		case *copyBodyOption:
			copyBody = o.copy
		case *subVersionOption:
			subVersion = o.subVersion
		}
	}

	// 校验版本是否受支持
	if !isSupportedVersion(v) {
		return nil, NewUnsupportedVersionError(v, SupportedVersions)
	}

	// 处理body拷贝
	var bodyData []byte
	if copyBody {
		bodyData = make([]byte, len(body))
		copy(bodyData, body)
	} else {
		bodyData = body
	}

	frame := &Frame{
		Version:    v,
		SubVersion: subVersion,
		Type:       frameType,
		bodyLength: uint32(len(bodyData)),
		Body:       bodyData,
	}

	return frame, nil
}

// Encode 编码协议帧
//
// 将Frame结构体编码为字节数组，用于网络传输
//
// 返回值：
//
//	 []byte - 成功时返回编码后的字节数组
//
//	error - 失败时返回错误信息
//
// 使用用例：
//
//  1. 编码普通帧
//     frame := NewFrame(FrameTypeJSON, []byte("hello"))
//     data, err := frame.Encode()
//     返回值：[1 1 0 0 0 5 104 101 108 108 111], nil (版本号1, 类型1, 消息体长度5, 消息体hello)
//
//  2. 编解码可逆性测试
//     originalFrame := NewFrame(FrameTypeJSON, []byte("test"))
//     data, _ := originalFrame.Encode()
//     decodedFrame, _ := Decode(data)
//     // decodedFrame 应与 originalFrame 相同
//
// 错误处理：
//
//  1. 消息体过长
//     当消息体长度超过MaxMessageLength（1MB）时
//     返回值: nil, ErrMessageTooLong
//
//  2. 不支持的协议版本
//     当版本号不是ProtocolVersionV1或ProtocolVersionV2时
//     返回值: nil, fmt.Errorf("%w: %d, supported versions: 1,2", ErrUnsupportedVersion, version)
//
// 实现中的重要细节：
//
//   - 使用大端序编码消息体长度
//   - 帧格式：[1字节版本号][1字节消息类型][4字节消息体长度][消息体]
//   - 总长度为帧头长度（6字节）加上消息体长度
func (f *Frame) Encode() ([]byte, error) {
	// 验证版本是否为支持的版本
	if !isSupportedVersion(f.Version) {
		return nil, NewUnsupportedVersionError(f.Version, SupportedVersions)
	}
	if len(f.Body) > MaxMessageLength {
		return nil, NewMessageTooLongError(len(f.Body), MaxMessageLength)
	}

	// 计算总长度：帧头长度 + 消息体长度
	totalLength := FrameHeaderLength + len(f.Body)

	// 从分级池中获取合适大小的缓冲区
	bufPtr := bufferPool.Get(totalLength)
	buf := *bufPtr

	// 如果是超大缓冲区（超过8KB），直接使用所需大小
	if cap(buf) < totalLength {
		buf = make([]byte, totalLength)
		*bufPtr = buf // 更新指针指向新缓冲区
	}

	// 重置缓冲区长度
	buf = buf[:totalLength]

	// 写入版本号
	buf[0] = f.Version
	// 写入子版本号
	buf[1] = f.SubVersion
	// 写入消息类型
	buf[2] = f.Type
	// 写入消息体长度
	binary.BigEndian.PutUint32(buf[3:7], f.bodyLength)
	// 写入消息体
	copy(buf[7:], f.Body)

	// 创建返回值副本，避免池中的缓冲区被修改
	result := make([]byte, totalLength)
	copy(result, buf)

	// 将缓冲区放回合适的池中
	bufferPool.Put(bufPtr)

	return result, nil
}

// EncodeTo 将帧编码并写入io.Writer
// 支持直接写入网络连接、文件等，避免中间缓冲区分配
//
// 参数：
//
//	w - 目标io.Writer
//
// 返回值：
//
//	n - 写入的字节数
//	err - 错误信息
//
// 错误处理：
//  1. 版本不支持：返回0, NewUnsupportedVersionError
//  2. 消息体过长：返回0, NewMessageTooLongError
//  3. 写入失败：返回已写入字节数, 具体io错误
func (f *Frame) EncodeTo(w io.Writer) (n int, err error) {
	// 验证版本是否为支持的版本
	if !isSupportedVersion(f.Version) {
		return 0, NewUnsupportedVersionError(f.Version, SupportedVersions)
	}
	if len(f.Body) > MaxMessageLength {
		return 0, NewMessageTooLongError(len(f.Body), MaxMessageLength)
	}

	// 构造帧头
	header := make([]byte, FrameHeaderLength)
	header[0] = f.Version
	header[1] = f.SubVersion
	header[2] = f.Type
	binary.BigEndian.PutUint32(header[3:7], f.bodyLength)

	// 写入帧头
	n, err = w.Write(header)
	if err != nil {
		return n, err
	}

	// 写入消息体
	var bodyN int
	bodyN, err = w.Write(f.Body)
	n += bodyN
	if err != nil {
		return n, err
	}

	return n, nil
}

// EncodeToBytes 将帧编码到提供的缓冲区
// 支持复用缓冲区，减少内存分配，适用于高频场景
//
// 参数：
//
//	buf - 目标缓冲区，必须足够大，至少需要 FrameHeaderLength + len(Body) 字节
//
// 返回值：
//
//	n - 写入的字节数
//	err - 错误信息
//
// 错误处理：
//  1. 版本不支持：返回0, NewUnsupportedVersionError
//  2. 消息体过长：返回0, NewMessageTooLongError
//  3. 缓冲区不足：返回0, errors.New("buffer too small")
//
// 使用示例：
//
//	// 预分配足够大的缓冲区
//	buf := make([]byte, FrameHeaderLength + len(frame.Body))
//	n, err := frame.EncodeToBytes(buf)
//	if err == nil {
//		// 使用 buf[:n] 作为编码结果
//	}
func (f *Frame) EncodeToBytes(buf []byte) (n int, err error) {
	// 验证版本是否为支持的版本
	if !isSupportedVersion(f.Version) {
		return 0, NewUnsupportedVersionError(f.Version, SupportedVersions)
	}
	if len(f.Body) > MaxMessageLength {
		return 0, NewMessageTooLongError(len(f.Body), MaxMessageLength)
	}

	// 计算总长度：帧头长度 + 消息体长度
	totalLength := FrameHeaderLength + len(f.Body)

	// 检查缓冲区大小是否足够
	if len(buf) < totalLength {
		return 0, &ProtocolError{
			Code:    ErrCodeBufferTooSmall,
			Message: fmt.Sprintf("buffer too small: need %d bytes, got %d bytes", totalLength, len(buf)),
		}
	}

	// 写入版本号
	buf[0] = f.Version
	// 写入子版本号
	buf[1] = f.SubVersion
	// 写入消息类型
	buf[2] = f.Type
	// 写入消息体长度
	binary.BigEndian.PutUint32(buf[3:7], f.bodyLength)
	// 写入消息体
	copy(buf[7:7+len(f.Body)], f.Body)

	return totalLength, nil
}

// Decode 解码协议帧
//
// 将字节数组解码为Frame结构体，用于解析网络接收的数据
//
// 参数：
//
//  data - 要解码的字节数组
//
// 返回值：
//
//  *Frame - 成功时返回解码后的Frame结构体指针
//
//	error - 失败时返回错误信息
//
// 使用用例：
//
//  1. 解码V1版本帧
//     data := []byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
//     frame, err := Decode(data)
//     返回值：&Frame{Version:1, Type:1, BodyLength:5, Body:[]byte("hello")}, nil
//
//  2. 解码边界情况（空消息体）
//     data := []byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x00}
//     frame, err := Decode(data)
//     返回值：&Frame{Version:1, Type:1, BodyLength:0, Body:[]byte{}}, nil
//
//  3. 编解码可逆性测试
//     originalData := []byte("test message")
//     originalFrame, _ := NewFrame(FrameTypeJSON, originalData)
//     encodedData, _ := originalFrame.Encode()
//     decodedFrame, _ := Decode(encodedData)
//     // bytes.Equal(originalFrame.Body, decodedFrame.Body) == true
//
// 错误处理：
//
//  1. 无效的帧格式
//     当输入数据长度小于帧头长度（6字节）时
//     返回值: nil, ErrInvalidFrame
//
//  2. 不支持的协议版本
//     当版本号不是ProtocolVersionV1或ProtocolVersionV2时
//     返回值: nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
//
//  3. 消息体不完整
//     当数据长度小于帧头长度+消息体长度时
//     返回值: nil, ErrInvalidFrame
//
//  4. 无效的帧类型
//     当帧类型不合法时
//     返回值: nil, ErrInvalidFrameType
//
// 实现中的重要细节：
//
//   - 使用大端序解码消息长度
//   - 版本字段在前1个字节，用于区分不同的协议版本
//   - 根据版本号直接调用对应的解码函数
func Decode(data []byte) (*Frame, error) {
	if len(data) < FrameHeaderLength {
		return nil, NewInvalidFrameError(fmt.Sprintf("data length %d is less than header length %d", len(data), FrameHeaderLength))
	}

	// 解析版本号
	version := data[0]

	// 根据版本号调用对应的解码函数
	switch version {
	case ProtocolVersionV1:
		return decodeV1(data)
	case ProtocolVersionV2:
		return decodeV2(data)
	default:
		return nil, NewUnsupportedVersionError(version, SupportedVersions)
	}
}

// decodeV1 解码V1版本的协议帧
// 帧格式：[1字节版本号][1字节消息类型][4字节消息体长度][消息体]
func decodeV1(data []byte) (*Frame, error) {
	// 解析子版本号
	subVersion := data[1]
	// 解析消息类型
	frameType := data[2]
	// 解析消息体长度
	bodyLength := binary.BigEndian.Uint32(data[3:7])

	// 检查数据是否完整
	expectedLength := FrameHeaderLength + int(bodyLength)
	if len(data) < expectedLength {
		return nil, NewInvalidFrameError(fmt.Sprintf("data length %d is less than expected %d (header + body)", len(data), expectedLength))
	}

	// 校验帧类型合法性
	if !isValidFrameType(frameType) {
		return nil, NewInvalidFrameTypeError(frameType, []uint8{FrameTypeJSON, FrameTypeProtobuf, FrameTypeMsgPack})
	}

	// 解析消息体并深拷贝，避免原始数据修改影响Frame
	body := make([]byte, bodyLength)
	copy(body, data[FrameHeaderLength:expectedLength])

	return &Frame{
		Version:    ProtocolVersionV1,
		SubVersion: subVersion,
		Type:       frameType,
		bodyLength: bodyLength,
		Body:       body,
	}, nil
}

// decodeV2 解码V2版本的协议帧
// 帧格式：[1字节版本号][1字节子版本号][1字节消息类型][4字节消息体长度][消息体]
func decodeV2(data []byte) (*Frame, error) {
	// 解析子版本号
	subVersion := data[1]
	// 解析消息类型
	frameType := data[2]
	// 解析消息体长度
	bodyLength := binary.BigEndian.Uint32(data[3:7])

	// 检查数据是否完整
	expectedLength := FrameHeaderLength + int(bodyLength)
	if len(data) < expectedLength {
		return nil, NewInvalidFrameError(fmt.Sprintf("data length %d is less than expected %d (header + body)", len(data), expectedLength))
	}

	// 校验帧类型合法性
	if !isValidFrameType(frameType) {
		return nil, NewInvalidFrameTypeError(frameType, []uint8{FrameTypeJSON, FrameTypeProtobuf, FrameTypeMsgPack})
	}

	// 解析消息体并深拷贝，避免原始数据修改影响Frame
	body := make([]byte, bodyLength)
	copy(body, data[FrameHeaderLength:expectedLength])

	return &Frame{
		Version:    ProtocolVersionV2,
		SubVersion: subVersion,
		Type:       frameType,
		bodyLength: bodyLength,
		Body:       body,
	}, nil
}

// Clone 创建Frame的深拷贝
// 返回一个新的Frame实例，包含完全相同的字段值，但Body是深拷贝的
func (f *Frame) Clone() *Frame {
	if f == nil {
		return nil
	}

	// 深拷贝Body
	body := make([]byte, len(f.Body))
	copy(body, f.Body)

	return &Frame{
		Version:    f.Version,
		SubVersion: f.SubVersion,
		Type:       f.Type,
		bodyLength: f.bodyLength,
		Body:       body,
	}
}

// String 返回Frame的字符串表示，适合日志输出
// 格式：Frame{Version:1, SubVersion:0, Type:1(JSON), BodyLength:18, Body:"{\"message\":\"hello\"}"}
// 注意：Body内容超过64字节时会被截断并添加省略号
func (f *Frame) String() string {
	// 转换帧类型为可读字符串
	typeStr := fmt.Sprintf("%d", f.Type)
	switch f.Type {
	case FrameTypeJSON:
		typeStr = "1(JSON)"
	case FrameTypeProtobuf:
		typeStr = "2(Protobuf)"
	case FrameTypeMsgPack:
		typeStr = "3(MsgPack)"
	}

	// 处理Body字符串，最多显示64字节
	bodyStr := string(f.Body)
	if len(bodyStr) > 64 {
		bodyStr = bodyStr[:64] + "..."
	}

	return fmt.Sprintf("Frame{Version:%d, SubVersion:%d, Type:%s, BodyLength:%d, Body:%q}",
		f.Version, f.SubVersion, typeStr, f.bodyLength, bodyStr)
}

// PrettyPrint 以十六进制和ASCII对照的形式输出Frame的完整内容
// 适合调试网络粘包、拆包等问题
//
// 输出格式示例：
// Frame Details:
//
//	Version: 1
//	SubVersion: 0
//	Type: 1(JSON)
//	BodyLength: 18
//	TotalLength: 25
//	Raw Data:
//	00000000  01 00 01 00 00 00 12 7b  22 6d 65 73 73 61 67 65  |.......{"message|
//	00000010  22 3a 22 68 65 6c 6c 6f  22 7d                    |":"hello"}|
//
// 参数：
//
//	w - 输出目标，如os.Stdout、strings.Builder等
func (f *Frame) PrettyPrint(w io.Writer) error {
	// 先编码Frame得到原始数据
	rawData, err := f.Encode()
	if err != nil {
		return err
	}

	// 转换帧类型为可读字符串
	typeStr := fmt.Sprintf("%d", f.Type)
	switch f.Type {
	case FrameTypeJSON:
		typeStr = "1(JSON)"
	case FrameTypeProtobuf:
		typeStr = "2(Protobuf)"
	case FrameTypeMsgPack:
		typeStr = "3(MsgPack)"
	}

	// 输出基本信息
	fmt.Fprintf(w, "Frame Details:\n")
	fmt.Fprintf(w, "  Version: %d\n", f.Version)
	fmt.Fprintf(w, "  SubVersion: %d\n", f.SubVersion)
	fmt.Fprintf(w, "  Type: %s\n", typeStr)
	fmt.Fprintf(w, "  BodyLength: %d\n", f.bodyLength)
	fmt.Fprintf(w, "  TotalLength: %d\n", len(rawData))
	fmt.Fprintf(w, "  Raw Data:\n")

	// 输出十六进制和ASCII对照
	for i := 0; i < len(rawData); i += 16 {
		end := i + 16
		if end > len(rawData) {
			end = len(rawData)
		}

		// 输出偏移量
		fmt.Fprintf(w, "  %08x  ", i)

		// 输出十六进制部分
		for j := i; j < end; j++ {
			fmt.Fprintf(w, "%02x ", rawData[j])
		}

		// 填充空格，保持对齐
		for j := end; j < i+16; j++ {
			fmt.Fprint(w, "   ")
		}

		// 输出ASCII部分
		fmt.Fprint(w, " |")
		for j := i; j < end; j++ {
			b := rawData[j]
			if b >= 32 && b <= 126 {
				fmt.Fprintf(w, "%c", b)
			} else {
				fmt.Fprint(w, ".")
			}
		}
		fmt.Fprintln(w, "|")
	}

	return nil
}

// streamDecoderPool 用于复用StreamDecoder实例的同步池
var streamDecoderPool = sync.Pool{
	New: func() interface{} {
		return &StreamDecoder{
			buffer:        make([]byte, 0, 1024), // 初始容量1KB
			maxBufferSize: MaxMessageLength + FrameHeaderLength,
		}
	},
}

// StreamDecoder 支持流式解码的结构体，用于处理TCP粘包/拆包场景
// 它维护一个内部缓冲区，可以接收不完整的数据，并在数据足够时解析出完整的帧
type StreamDecoder struct {
	// buffer 存储接收到的数据
	buffer []byte
	// maxBufferSize 缓冲区最大大小，防止内存耗尽攻击
	maxBufferSize int
}

// NewStreamDecoder 从池中获取StreamDecoder实例
// 使用完后应该调用Release方法放回池中
func NewStreamDecoderFromPool(maxBufferSize ...int) *StreamDecoder {
	sd := streamDecoderPool.Get().(*StreamDecoder)

	// 如果指定了最大缓冲区大小，更新它
	if len(maxBufferSize) > 0 && maxBufferSize[0] > 0 {
		sd.maxBufferSize = maxBufferSize[0]
	}

	return sd
}

// Release 将解码器放回池中，以便重用
// 在不再需要解码器时调用此方法，而不是直接丢弃
func (sd *StreamDecoder) Release() {
	// 重置缓冲区，但不释放到池中，因为解码器本身会被重用
	sd.buffer = sd.buffer[:0]

	// 将解码器放回池中
	streamDecoderPool.Put(sd)
}

// NewStreamDecoder 创建一个新的StreamDecoder实例
// maxBufferSize: 缓冲区最大大小，默认为MaxMessageLength + FrameHeaderLength
func NewStreamDecoder(maxBufferSize ...int) *StreamDecoder {
	maxSize := MaxMessageLength + FrameHeaderLength
	if len(maxBufferSize) > 0 && maxBufferSize[0] > 0 {
		maxSize = maxBufferSize[0]
	}

	return &StreamDecoder{
		buffer:        make([]byte, 0, 1024), // 初始容量1KB
		maxBufferSize: maxSize,
	}
}

// Feed 向解码器提供数据
// 这些数据会被追加到内部缓冲区中
// 返回错误如果缓冲区大小超过限制
func (sd *StreamDecoder) Feed(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// 检查添加数据后是否会超过缓冲区大小限制
	if len(sd.buffer)+len(data) > sd.maxBufferSize {
		return NewMessageTooLongError(len(sd.buffer)+len(data), sd.maxBufferSize)
	}

	// 确保缓冲区有足够容量
	if cap(sd.buffer)-len(sd.buffer) < len(data) {
		// 计算新容量，至少是当前容量的2倍或足够容纳新数据
		newCap := cap(sd.buffer) * 2
		if newCap < len(sd.buffer)+len(data) {
			newCap = len(sd.buffer) + len(data)
		}

		// 限制最大容量
		if newCap > sd.maxBufferSize {
			newCap = sd.maxBufferSize
		}

		// 从缓冲区池获取合适大小的缓冲区
		bufPtr := bufferPool.Get(newCap)
		newBuf := *bufPtr

		// 重置新缓冲区的长度为0，确保数据从开头开始写入
		newBuf = newBuf[:0]

		// 复制现有数据
		newBuf = append(newBuf, sd.buffer...)

		// 释放旧缓冲区（如果它来自池）
		if cap(sd.buffer) == smallBufferSize || cap(sd.buffer) == mediumBufferSize || cap(sd.buffer) == largeBufferSize {
			// 如果旧缓冲区是池中的大小，放回池中
			oldBufPtr := &sd.buffer
			bufferPool.Put(oldBufPtr)
		}

		// 使用新缓冲区
		sd.buffer = newBuf
	}

	// 追加新数据
	sd.buffer = append(sd.buffer, data...)
	return nil
}

// TryDecode 尝试从缓冲区中解码一个完整的帧
// 如果缓冲区中没有足够的数据，返回nil, nil
// 如果有足够数据，返回解码的帧和更新后的缓冲区
// 如果数据格式错误，返回nil, error
func (sd *StreamDecoder) TryDecode() (*Frame, error) {
	// 检查是否有足够的数据读取帧头
	if len(sd.buffer) < FrameHeaderLength {
		return nil, nil // 数据不足，等待更多数据
	}

	// 读取版本号
	version := sd.buffer[0]

	// 检查版本是否支持
	if !isSupportedVersion(version) {
		return nil, NewUnsupportedVersionError(version, SupportedVersions)
	}

	// 读取消息体长度
	bodyLength := binary.BigEndian.Uint32(sd.buffer[3:7])

	// 检查消息体长度是否合法
	if bodyLength > uint32(MaxMessageLength) {
		return nil, NewMessageTooLongError(int(bodyLength), MaxMessageLength)
	}

	// 计算完整帧的长度
	frameLength := FrameHeaderLength + int(bodyLength)

	// 检查是否有足够的数据读取完整帧
	if len(sd.buffer) < frameLength {
		return nil, nil // 数据不足，等待更多数据
	}

	// 提取完整的帧数据
	frameData := sd.buffer[:frameLength]

	// 更新缓冲区，移除已处理的数据
	// 优化：避免内存泄漏，当缓冲区大小远大于剩余数据时，重新分配
	remaining := len(sd.buffer) - frameLength
	if remaining > 0 && remaining < cap(sd.buffer)/4 {
		// 当剩余数据小于容量的1/4时，重新分配以释放内存
		newBuf := make([]byte, remaining)
		copy(newBuf, sd.buffer[frameLength:])
		sd.buffer = newBuf
	} else {
		sd.buffer = sd.buffer[frameLength:]
	}

	// 使用现有的Decode函数解码帧
	return Decode(frameData)
}

// DecodeFromReader 从io.Reader中读取数据并尝试解码帧
// 这是一个便利方法，结合了Feed和TryDecode操作
// 返回解码的帧和可能的错误
func (sd *StreamDecoder) DecodeFromReader(reader io.Reader) (*Frame, error) {
	// 尝试从缓冲区解码现有数据
	frame, err := sd.TryDecode()
	if err != nil {
		return nil, err
	}

	if frame != nil {
		return frame, nil
	}

	// 如果缓冲区中没有完整帧，尝试从reader读取更多数据
	// 使用池化的缓冲区，减少内存分配
	tempBufPtr := bufferPool.Get(1024)
	defer bufferPool.Put(tempBufPtr)
	tempBuf := *tempBufPtr

	n, err := reader.Read(tempBuf)
	if err != nil {
		if err == io.EOF {
			// 如果缓冲区为空，返回EOF
			if sd.Buffered() == 0 {
				return nil, io.EOF
			}
			// 如果缓冲区中有数据但不完整，返回nil表示没有完整帧
			return nil, nil
		}
		return nil, err
	}

	// 将读取的数据添加到缓冲区
	if err := sd.Feed(tempBuf[:n]); err != nil {
		return nil, err
	}

	// 再次尝试解码
	return sd.TryDecode()
}

// Reset 重置解码器的内部缓冲区
// 在连接错误或需要重新开始解码时使用
func (sd *StreamDecoder) Reset() {
	// 如果缓冲区来自池，将其放回池中
	if cap(sd.buffer) == smallBufferSize || cap(sd.buffer) == mediumBufferSize || cap(sd.buffer) == largeBufferSize {
		bufPtr := &sd.buffer
		bufferPool.Put(bufPtr)
	}

	// 创建一个新的小缓冲区，减少内存占用
	sd.buffer = make([]byte, 0, smallBufferSize)
}

// Buffered 返回当前缓冲区中的数据量
func (sd *StreamDecoder) Buffered() int {
	return len(sd.buffer)
}

// Peek 返回当前缓冲区中的数据副本，不消费数据
// 主要用于调试
func (sd *StreamDecoder) Peek() []byte {
	// 使用池化的缓冲区，减少内存分配
	if len(sd.buffer) == 0 {
		return nil
	}

	bufPtr := bufferPool.Get(len(sd.buffer))
	defer bufferPool.Put(bufPtr)
	buf := *bufPtr

	// 确保缓冲区大小足够
	if cap(buf) < len(sd.buffer) {
		buf = make([]byte, len(sd.buffer))
	} else {
		buf = buf[:len(sd.buffer)]
	}

	// 复制数据
	copy(buf, sd.buffer)

	// 创建返回的副本，避免池化缓冲区被修改
	result := make([]byte, len(buf))
	copy(result, buf)
	return result
}

// ReadFramesFromStream 从流中连续读取所有可解码的帧
// 直到遇到错误或EOF
// 返回解码的帧列表和可能的错误
func (sd *StreamDecoder) ReadFramesFromStream(reader io.Reader) ([]*Frame, error) {
	// 使用预分配的切片，减少内存重新分配
	frames := make([]*Frame, 0, 16) // 预分配容量，减少扩容

	for {
		frame, err := sd.DecodeFromReader(reader)
		if err != nil {
			if err == io.EOF {
				return frames, nil
			}
			return frames, err
		}

		if frame != nil {
			frames = append(frames, frame)
		} else {
			// 没有解码出帧，但也没有错误，可能需要更多数据
			// 如果已经读取了所有数据，退出循环
			if sd.Buffered() == 0 {
				break
			}
		}
	}

	return frames, nil
}

// WriteTo 将解码器内部缓冲区的内容写入到指定的io.Writer
// 主要用于调试或数据转移
func (sd *StreamDecoder) WriteTo(w io.Writer) (int64, error) {
	if len(sd.buffer) == 0 {
		return 0, nil
	}

	n, err := w.Write(sd.buffer)
	return int64(n), err
}

// IsEmpty 检查解码器缓冲区是否为空
func (sd *StreamDecoder) IsEmpty() bool {
	return len(sd.buffer) == 0
}

// Bytes 返回内部缓冲区的引用，不进行拷贝
// 注意：调用者不应修改返回的字节切片
func (sd *StreamDecoder) Bytes() []byte {
	return sd.buffer
}

