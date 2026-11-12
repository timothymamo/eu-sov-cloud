package main

import "unsafe"

// unsafePointers takes a pointer apart, which a raw system call is handed an
// address with. The groups hold for the server composition root's _test.go files
// too, as they hold for the CLI's.
func unsafePointers() {
	_ = unsafe.Pointer(nil) // want "composition-root-no-exec: "
	_ = unsafe.Sizeof(0)    // want "composition-root-no-exec: "
}
