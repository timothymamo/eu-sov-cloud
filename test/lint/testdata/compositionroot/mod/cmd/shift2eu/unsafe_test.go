package main

import "unsafe"

// unsafePointers takes a pointer apart, which a raw system call is handed an
// address with. The groups hold for a composition root's _test.go files too: a
// test is compiled into the same package and runs with the same environment, and
// depguard's composition-root rule skips _test.go files, so this line is what
// keeps package unsafe out of them.
func unsafePointers() {
	_ = unsafe.Pointer(nil) // want "composition-root-no-exec: "
	_ = unsafe.Sizeof(0)    // want "composition-root-no-exec: "
}
