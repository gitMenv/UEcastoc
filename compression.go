package main

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io/ioutil"
	"strings"

	"github.com/new-world-tools/go-oodle"
)

// implemented (de)compression methods (lowercased)

var (
	DecompressionMethods = map[string](func(*[]byte, uint32) (*[]byte, error)){
		"none":  decompressNone,
		"zlib":  decompressZLIB,
		"oodle": decompressOodle,
	}
	CompressionMethods = map[string](func(*[]byte) (*[]byte, error)){
		"none":  compressNone,
		"zlib":  compressZLIB,
		"oodle": compressOodle, // settings: level 3 Kraken compression
	}
)

/* Decompression functions */
func decompressNone(inData *[]byte, expectedOutputSize uint32) (*[]byte, error) {
	return inData, nil // can't go wrong :D
}

func decompressZLIB(inData *[]byte, expectedOutputSize uint32) (*[]byte, error) {
	// decompress with zlib
	r, err := zlib.NewReader(bytes.NewBuffer(*inData))
	defer r.Close()
	if err != nil {
		return nil, err
	}
	uncompressed, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(uncompressed) != int(expectedOutputSize) {
		return nil, errors.New("zlib did not decompress correctly")
	}
	return &uncompressed, nil
}

func decompressOodle(inData *[]byte, expectedOutputSize uint32) (*[]byte, error) {
	if !oodle.IsDllExist() {
		err := oodle.Download()
		if err != nil {
			return nil, errors.New("oo2core_9_win64.dll was not found (oodle decompression)")
		}
	}
	output, err := oodle.Decompress(*inData, int64(expectedOutputSize))
	// if err is not nil, it's handled by the caller
	return &output, err
}

/* Compression functions */

func compressNone(inData *[]byte) (*[]byte, error) {
	return inData, nil
}
func compressZLIB(inData *[]byte) (*[]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(*inData)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	compressedData := b.Bytes()
	return &compressedData, nil
}
func compressOodle(inData *[]byte) (*[]byte, error) {
	// The settings for Oodle _could_ be modified, but this is what Unreal Engine states as example
	// https://docs.unrealengine.com/4.27/en-US/TestingAndOptimization/Oodle/Data/
	compressedData, err := oodle.Compress(*inData, oodle.AlgoKraken, oodle.CompressionLevelOptimal3)
	return &compressedData, err
}

/* Wrapper for getting the functions */
// depending on the method, return the associated decompression function
func getDecompressionFunction(method string) func(inData *[]byte, outputSize uint32) (*[]byte, error) {
	if val, ok := DecompressionMethods[strings.ToLower(method)]; ok {
		return val
	}
	return nil
}
func getCompressionFunction(method string) func(inData *[]byte) (*[]byte, error) {
	if val, ok := CompressionMethods[strings.ToLower(method)]; ok {
		return val
	}
	return nil
}
