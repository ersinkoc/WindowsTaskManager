//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func main() {
	// Lone high surrogate
	s := "\uD800"
	p, err := windows.UTF16PtrFromString(s)
	fmt.Printf("Lone high surrogate: ptr=%p err=%v\n", p, err)

	// Unpaired low surrogate
	s2 := "\uDC00"
	p2, err2 := windows.UTF16PtrFromString(s2)
	fmt.Printf("Lone low surrogate: ptr=%p err=%v\n", p2, err2)

	// Valid surrogate pair
	s3 := "\uD83D\uDE00" // 😀
	p3, err3 := windows.UTF16PtrFromString(s3)
	fmt.Printf("Valid pair: ptr=%p err=%v\n", p3, err3)

	// Embedded NUL
	s4 := "hello\x00world"
	p4, err4 := windows.UTF16PtrFromString(s4)
	fmt.Printf("Embedded NUL: ptr=%p err=%v\n", p4, err4)
}
