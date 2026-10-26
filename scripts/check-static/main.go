// 静态ELF验证工具 / Static ELF Validator
// 功能：开发与CI校验Linux可执行产物，不执行输入文件，不进入发布程序
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 22:09:19
package main

import (
	"debug/elf"
	"fmt"
	"os"
)

func verify(path string) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open ELF: %w", err)
	}
	defer func() { _ = file.Close() }()
	if file.Type != elf.ET_EXEC {
		return fmt.Errorf("expected an executable ELF, got %s", file.Type)
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP || program.Type == elf.PT_DYNAMIC {
			return fmt.Errorf("dynamic program header: %s", program.Type)
		}
	}
	libraries, err := file.ImportedLibraries()
	if err != nil {
		return fmt.Errorf("read shared libraries: %w", err)
	}
	if len(libraries) != 0 {
		return fmt.Errorf("shared libraries remain: %v", libraries)
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: check-static <linux-elf> [...]")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := verify(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("verified static ELF: %s\n", path)
	}
}
