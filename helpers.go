package main

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unsafe"
)
import "C"

func strSliceToC(list *[]string) **C.char {
	strlist := *list
	n := len(strlist)
	// create an unsafe pointer with sufficient space on the stack
	strs := C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(uintptr(0))))
	// fill the array with strings by offsetting the unsafe pointers with the size of the char pointers
	for i := 0; i < int(n); i++ {
		*(**C.char)(unsafe.Pointer(uintptr(strs) + uintptr(i)*unsafe.Sizeof(*(**C.char)(strs)))) = C.CString(strlist[i])
	}
	return (**C.char)(strs)
}

func convertAES(AES *C.char) []byte {
	s := ""
	if AES != nil {
		s = C.GoString(AES)
	}
	// go string to []byte
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")

	b, _ := hex.DecodeString(s)
	return b
}

func decryptAES(ciphertext *[]byte, AES []byte) (*[]byte, error) {
	block, err := aes.NewCipher(AES)
	if err != nil {
		return nil, err
	}
	dst := make([]byte, len(*ciphertext))
	for i := 0; i < len(dst); i += block.BlockSize() {
		block.Decrypt(dst[i:], (*ciphertext)[i:])
	}
	return &dst, nil
}
func encryptAES(plaintext *[]byte, AES []byte) (*[]byte, error) {
	block, err := aes.NewCipher(AES)
	if err != nil {
		return nil, err
	}
	dst := make([]byte, len(*plaintext))
	for i := 0; i < len(dst); i += block.BlockSize() {
		block.Encrypt(dst[i:], (*plaintext)[i:])
	}
	return &dst, nil
}

func sha1Hash(fdata *[]byte) *FIoChunkHash {
	hasher := sha1.New()
	hasher.Write(*fdata)
	fileHash := hasher.Sum(nil)

	var hash FIoChunkHash
	copy(hash.Hash[:], fileHash[:20])
	hash.Padding = [12]byte{} // explicitly set to 0
	return &hash
}

func getRandomBytes(n int) []byte {
	ret := make([]byte, n)
	rand.Read(ret)
	return ret
}

// A string must have a preamble of the strlen and a nullbyte at the end.
// this function returns the string in the "FString" format.
func stringToFString(str string) []byte {
	strlen := uint32(len(str) + 1) // include nullbyte
	fstring := make([]byte, int(strlen)+binary.Size(strlen))
	binary.LittleEndian.PutUint32(fstring, strlen)
	for i := 0; i < len(str); i++ {
		fstring[4+i] = str[i]
	}
	fstring[len(fstring)-1] = 0
	return fstring
}

func uint32ToBytes(a *uint32) *[]byte {
	t := make([]byte, 4)
	binary.LittleEndian.PutUint32(t, *a)
	return &t
}
