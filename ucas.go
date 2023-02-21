package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

func (d *UTocData) unpackFile(fdata *GameFileMetaData, blockData *[][]byte, outDir string) error {
	os.MkdirAll(outDir, 0700)
	bdata := *blockData
	outputData := []byte{}
	for i := 0; i < len(bdata); i++ {
		method := d.compressionMethods[fdata.compressionBlocks[i].CompressionMethod]
		decomp := getDecompressionFunction(method)
		if decomp == nil {
			return errors.New(fmt.Sprintf("decompression method %s not known", method))
		}
		newData, err := decomp(&(bdata[i]), fdata.compressionBlocks[i].GetUncompressedSize())
		if err != nil {
			return err
		}
		outputData = append(outputData, (*newData)...)
	}
	// ensure path exists to the file
	fpath := filepath.Clean(outDir + fdata.filepath)
	directory := filepath.Dir(fpath)

	os.MkdirAll(directory, 0700)
	// write the actual data to the new file
	err := os.WriteFile(fpath, outputData, 0644)

	return err
}

func (d *UTocData) matchRegex(regex string) *[]GameFileMetaData {
	filesToUnpack := []GameFileMetaData{}
	for _, v := range d.files {
		match, err := regexp.MatchString(regex, v.filepath)
		if err != nil {
			return &filesToUnpack
		}
		// exclude special "dependencies" file, as it's not meant to be directly unpacked
		// for unpacking that file, have a look at the function to construct the manifest!
		if match && v.filepath != DepFileName {
			filesToUnpack = append(filesToUnpack, v)
		}
	}
	return &filesToUnpack
}

func (d *UTocData) unpackUcasFiles(ucasPath string, outDir string, regex string) (filesUnpacked int, err error) {
	outDir += d.mountPoint // adjust for mountpoint
	filesUnpacked = 0
	// read the file
	openUcas, err := os.Open(ucasPath)
	if err != nil {
		return filesUnpacked, err
	}
	defer openUcas.Close()

	filesToUnpack := *(d.matchRegex(regex))
	// each "file" is built from compression blocks
	// extract those compression blocks from the .ucas file and use those for unpacking
	// Since there's one place where the .ucas file is actually read, it can act as a work divider.
	// that may make it possible to make it run multithreaded in the future!
	for _, v := range filesToUnpack {
		var compressionblockData [][]byte
		for _, b := range v.compressionBlocks {
			_, err = openUcas.Seek(int64(b.GetOffset()), 0)
			if err != nil {
				return filesUnpacked, err
			}
			buf := make([]byte, b.GetCompressedSize())
			readBytes, err := openUcas.Read(buf)
			if err != nil {
				return filesUnpacked, err
			}
			if uint32(readBytes) != b.GetCompressedSize() {
				return filesUnpacked, errors.New("could not read the correct size")
			}
			compressionblockData = append(compressionblockData, buf)
		}
		// all separate blocks collected for file unpacking
		err = d.unpackFile(&v, &compressionblockData, outDir)
		if err != nil {
			return filesUnpacked, err
		}
		filesUnpacked++
	}
	return len(filesToUnpack), nil
}
