// 静态产物护栏测试 / Static Artifact Guard Tests
// 功能：用ELF元数据样本验证动态/损坏产物拒绝，实际产物启动另由CI与容量验收验证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 22:09:19
package main

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func metadataELF(kind elf.Type, program elf.ProgType) []byte {
	raw := make([]byte, 120)
	copy(raw, "\x7fELF\x02\x01\x01")
	binary.LittleEndian.PutUint16(raw[16:], uint16(kind))
	binary.LittleEndian.PutUint16(raw[18:], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(raw[20:], 1)
	binary.LittleEndian.PutUint64(raw[32:], 64)
	binary.LittleEndian.PutUint16(raw[52:], 64)
	binary.LittleEndian.PutUint16(raw[54:], 56)
	binary.LittleEndian.PutUint16(raw[56:], 1)
	binary.LittleEndian.PutUint32(raw[64:], uint32(program))
	return raw
}

func TestVerifyArtifactLinkage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    []byte
		wantOK bool
	}{
		{"static_metadata", metadataELF(elf.ET_EXEC, elf.PT_LOAD), true},
		{"interpreter", metadataELF(elf.ET_EXEC, elf.PT_INTERP), false},
		{"dynamic_segment", metadataELF(elf.ET_EXEC, elf.PT_DYNAMIC), false},
		{"shared_object", metadataELF(elf.ET_DYN, elf.PT_LOAD), false},
		{"relocatable", metadataELF(elf.ET_REL, elf.PT_LOAD), false},
		{"truncated_headers", metadataELF(elf.ET_EXEC, elf.PT_LOAD)[:80], false},
		{"not_elf", []byte("not an executable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact")
			if err := os.WriteFile(path, tc.raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := verify(path); (err == nil) != tc.wantOK {
				t.Fatalf("verify error=%v, wantOK=%v", err, tc.wantOK)
			}
		})
	}
	if verify(filepath.Join(t.TempDir(), "missing")) == nil {
		t.Fatal("missing artifact accepted")
	}
}
