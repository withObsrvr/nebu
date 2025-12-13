package main

import (
	"fmt"
	"io"
	"os"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <xdr-file>\n", os.Args[0])
		os.Exit(1)
	}

	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var ledger xdr.LedgerCloseMeta
	_, err = xdr.Unmarshal(file, &ledger)
	if err != nil {
		if err == io.EOF {
			fmt.Println("Empty file")
		} else {
			fmt.Fprintf(os.Stderr, "Error unmarshaling: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("Ledger version: V%d\n", ledger.V)
	fmt.Printf("Ledger sequence: %d\n", ledger.LedgerSequence())

	// Show first few bytes of the file
	file.Seek(0, 0)
	buf := make([]byte, 32)
	n, _ := file.Read(buf)
	fmt.Printf("First %d bytes (hex): ", n)
	for i := 0; i < n; i++ {
		fmt.Printf("%02x ", buf[i])
		if (i+1)%16 == 0 {
			fmt.Printf("\n                      ")
		}
	}
	fmt.Println()
}
