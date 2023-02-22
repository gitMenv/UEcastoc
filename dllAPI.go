package main

// #include <stdio.h>
// #include <stdlib.h>
import "C"
import (
	"embed" // for the .pak file
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// The functions that use cgo stuff from C libraries must be used in the file
// that contains the main function.
var staticErr string

//go:embed req/Packed_P.pak
var embeddedFiles embed.FS

//export packGameFiles
func packGameFiles(dirPath *C.char, manifestPath *C.char, outFile *C.char, compressionMethod *C.char, AESKey *C.char) C.int {
	dir := C.GoString(dirPath)
	dir, err := filepath.Abs(dir)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	manifestFile := C.GoString(manifestPath)
	outPath := C.GoString(outFile)
	outPath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) // remove any extension
	compression := "None"
	if compressionMethod != nil {
		compression = C.GoString(compressionMethod)
	}
	aes := convertAES(AESKey)
	if len(aes) != 0 && len(aes) != 32 {
		staticErr = "AES key length should be 32, or none at all"
	}
	manifest, err := readManifest(manifestFile)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	n, err := packToCasToc(dir, manifest, outPath, compression, aes)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	// write the embedded .pak file
	embedded, _ := embeddedFiles.ReadFile("req/Packed_P.pak")
	os.WriteFile(outPath+".pak", embedded, os.ModePerm)
	return C.int(n - 1) // correction for dependencies file
}

//export freeStringList
func freeStringList(stringlist **C.char, n C.int) {
	for i := 0; i < int(n); i++ {
		toFreeString := *(**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(stringlist)) + uintptr(i)*unsafe.Sizeof(*stringlist)))
		C.free(unsafe.Pointer(toFreeString))
	}
	C.free(unsafe.Pointer(stringlist))
}

//export listGameFiles
func listGameFiles(utocFile *C.char, n *C.int, AESKey *C.char) (strlist **C.char) {
	utocFname := C.GoString(utocFile)
	aes := convertAES(AESKey)

	d, err := parseUtocFile(utocFname, aes)
	if err != nil {
		staticErr = err.Error()
		*n = C.int(-1)
		return nil
	}

	filepaths := []string{}
	for _, v := range d.files {
		if v.filepath == DepFileName {
			continue
		}
		filepaths = append(filepaths, v.filepath)
	}
	// each line a new string
	*n = C.int(len(filepaths))
	return strSliceToC(&filepaths)
}

//export getError
func getError() (err *C.char) {
	return C.CString(staticErr)
}

//export createManifestFile
func createManifestFile(utocFile *C.char, ucasFile *C.char, outputFile *C.char, AESKey *C.char) C.int {
	//TODO: check if the "dependencies" part works for more games, and if it's even required.
	utocFname := C.GoString(utocFile)
	ucasFname := C.GoString(ucasFile)
	outputFname := C.GoString(outputFile)
	aes := convertAES(AESKey)

	d, err := parseUtocFile(utocFname, aes)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	
	if d.hdr.isEncrypted(){
		tmpFile, err := os.CreateTemp("", "tmp")
		if err != nil {
			staticErr = err.Error()
			return C.int(-1)
		}
		ucasBytes, err := ioutil.ReadFile(ucasFname)
		if err != nil {
			staticErr = err.Error()
			return C.int(-1)
		}
		decryptedBytes, err := decryptAES(&ucasBytes, aes)
		if err != nil {
			staticErr = err.Error()
			return C.int(-1)
		}
		tmpFile.Write(*decryptedBytes)
		ucasFname = tmpFile.Name()
		err = tmpFile.Close()
		if err != nil {
			fmt.Println("err:", err)
			return C.int(-1)
		}
		defer os.Remove(tmpFile.Name())
	}
	manifest, err := d.constructManifest(ucasFname)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	b, err := json.MarshalIndent(manifest, "", "  ") // indent for readability
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	err = ioutil.WriteFile(outputFname, b, fs.ModePerm)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}

	return C.int(0)
}

//export unpackAllGameFiles
func unpackAllGameFiles(utocFile *C.char, ucasFile *C.char, outputDirectory *C.char, AESKey *C.char) C.int {
	reg := C.CString("/*")
	x := unpackGameFiles(utocFile, ucasFile, outputDirectory, reg, AESKey)
	C.free(unsafe.Pointer(reg)) // free the string that I made myself
	return x
}

//export unpackGameFiles
func unpackGameFiles(utocFile *C.char, ucasFile *C.char, outputDirectory *C.char, regex *C.char, AESKey *C.char) C.int {
	utocFname := C.GoString(utocFile)
	ucasFname := C.GoString(ucasFile)
	outDir := C.GoString(outputDirectory)
	reg := C.GoString(regex)
	aes := convertAES(AESKey)

	d, err := parseUtocFile(utocFname, aes)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	// ucas may also be encrypted; create temporary file and place decrypted version there
	// let the ucasreader read from the temporary file
	if d.hdr.isEncrypted() {
		tmpFile, err := os.CreateTemp("", "tmp")
		if err != nil {
			staticErr = err.Error()
			return C.int(-1)
		}
		ucasBytes, err := ioutil.ReadFile(ucasFname)
		if err != nil {
			staticErr = err.Error()
			return C.int(-1)
		}
		decryptedBytes, err := decryptAES(&ucasBytes, aes)
		if err != nil {
			staticErr = err.Error()
			return C.int(-1)
		}
		tmpFile.Write(*decryptedBytes)
		ucasFname = tmpFile.Name()
		err = tmpFile.Close()
		if err != nil {
			fmt.Println("err:", err)
			return C.int(-1)
		}
		defer os.Remove(tmpFile.Name())
	}

	// we need the parsed .utoc file to unpack the files that are included in the .ucas file.
	numberOfFiles, err := d.unpackUcasFiles(ucasFname, outDir, reg)
	if err != nil {
		staticErr = err.Error()
		return C.int(-1)
	}
	return C.int(numberOfFiles)
}

// main function is required for creating a DLL.
func main() {}
