package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"sort"
)

// This file is used to extract dependency data from the .ucas file.
// Not all fields are completely clear to me, so some of this must be saved somewhere to reconstruct the file later.
// I can imagine that if the dependencies are missing, something will go wrong.
// In short, this is "MAGIC" and I have no idea how or why it even works.

const DepFileName = "dependencies"

// Dependencies contains data extracted from the dependencies section in the .ucas file.
// An instance of this will be used to convert back to this section.
type Dependencies struct {
	ThisPackageID         uint64 `json:"packageID"`
	ChunkIDToDependencies map[uint64]FileDependency
}

type FileDependency struct {
	FileSize      uint64   `json:"uncompressedSize"`
	ExportObjects uint32   `json:"exportObjects"`
	MostlyOne     uint32   `json:"requiredValueSomehow"`
	SomeIndex     uint64   `json:"uniqueIndex"`
	Dependencies  []uint64 `json:"dependencies"` // lists the ID of each dependency
}

type DepsHeader struct {
	ThisPackageID    uint64
	NumberOfIDs      uint64
	IDSize           uint32  // always 8
	Padding          [4]byte // value is in hex, LE, 00 00 64 C1
	ZeroBytes        [4]byte
	NumberOfIDsAgain uint32 // whether this means an offset or?
}
type DepLinks struct {
	FileSize      uint64 // uncompressed file size
	ExportObjects uint32 // number of Export Objects
	MostlyOne     uint32 // this is either 1, 2 or 3, but most often 1.

	// All entries have a unique value for this field, starting at 0
	// It does skip some numbers, so I'm not sure what this means.
	// Looks like some kind of index though
	SomeIndex          uint64
	DependencyPackages uint32 // number of dependency packages that this connection has
	Offset             uint32 // an offset to point to the dependency packages
}

type parseDependencies struct {
	Hdr              DepsHeader
	IDs              []uint64
	FileLength       uint32 // file length from this offset
	Conns            []DepLinks
	OffsetAfterConss int64
	Deps             []uint64
	IDToConn         map[uint64]DepLinks
}

type Manifest struct {
	Files []ManifestFile `json:"Files,omitempty"` // in the .utoc file
	Deps  Dependencies   `json:"Dependencies,omitempty"`
	// Packages []UcasPackages `json:"Packages,omitempty"` // the "dependencies" in .ucas file???
}
type UcasPackages struct {
	PathName             string   `json:"Name"`
	ExportBundleChunkIds []string `json:"ExportBundleChunkIds,omitempty"`
	BulkDataChunkIds     []string `json:"BulkDataChunkIds,omitempty"`
}
type ManifestFile struct {
	Filepath string `json:"Path"`
	ChunkID  string `json:"ChunkId"`
}

func (u *UTocData) constructManifest(ucasPath string) (m Manifest, err error) {
	for _, v := range u.files {
		mf := ManifestFile{Filepath: v.filepath, ChunkID: v.chunkID.ToHexString()}
		m.Files = append(m.Files, mf)
	}
	// files part has been added, now decode the dependencies
	data, err := u.unpackDependencies(ucasPath)
	if err != nil {
		return m, err
	}
	x, err := ParseDependencies(*data)
	m.Deps = *x
	return m, err
}

func readManifest(manifestPath string) (*Manifest, error) {
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	err = json.Unmarshal(b, &manifest)
	return &manifest, err
}

func (s *parseDependencies) extractDependencies() *Dependencies {
	d := Dependencies{}
	d.ThisPackageID = s.Hdr.ThisPackageID
	d.ChunkIDToDependencies = make(map[uint64]FileDependency)
	for i, id := range s.IDs {
		fd := FileDependency{
			FileSize:      s.Conns[i].FileSize,
			ExportObjects: s.Conns[i].ExportObjects,
			MostlyOne:     s.Conns[i].MostlyOne,
			SomeIndex:     s.Conns[i].SomeIndex,
		}
		idx := s.Conns[i].Offset / 8
		for j := 0; j < int(s.Conns[i].DependencyPackages); j++ {
			fd.Dependencies = append(fd.Dependencies, s.Deps[idx+uint32(j)])
		}
		d.ChunkIDToDependencies[id] = fd
	}
	return &d
}

// Deparses the Dependencies struct exactly as how it was parsed
// This was checked using a simple diff tool.
func (d *Dependencies) Deparse() *[]byte {
	// deparse by writing all file portions to this buffer
	buf := bytes.NewBuffer([]byte{})

	// write hdr
	hdr := DepsHeader{
		ThisPackageID:    d.ThisPackageID,
		NumberOfIDs:      uint64(len(d.ChunkIDToDependencies)),
		IDSize:           8,
		Padding:          [4]byte{0x00, 0x00, 0x64, 0xC1},
		ZeroBytes:        [4]byte{},
		NumberOfIDsAgain: uint32(len(d.ChunkIDToDependencies)),
	}
	binary.Write(buf, binary.LittleEndian, hdr)

	// write list of IDs
	ids := []uint64{}
	totalNumberOfDependencies := 0
	for k := range d.ChunkIDToDependencies {
		ids = append(ids, k)
		totalNumberOfDependencies += len(d.ChunkIDToDependencies[k].Dependencies)
	}
	// these IDs are stored in order in this file, so sort here and use this ordering
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		binary.Write(buf, binary.LittleEndian, id)
	}
	// write file length from this point onwards
	var x DepLinks
	var flength uint32 = uint32(len(d.ChunkIDToDependencies)*binary.Size(x) + totalNumberOfDependencies*8)
	binary.Write(buf, binary.LittleEndian, flength)

	// write list of DepLinks entries
	endOfDeps := flength - uint32(totalNumberOfDependencies*8)
	depsToWrite := []uint64{}

	for i, id := range ids {
		entry := d.ChunkIDToDependencies[id]
		link := DepLinks{
			FileSize:           entry.FileSize,
			ExportObjects:      entry.ExportObjects,
			MostlyOne:          entry.MostlyOne,
			SomeIndex:          entry.SomeIndex,
			DependencyPackages: uint32(len(entry.Dependencies)),
			Offset:             0, // 0 by default, must be calculated otherwise
		}

		if link.DependencyPackages != 0 {
			// some math to write the correct offsets
			offsetFieldOffset := i*binary.Size(link) + 16 + 8
			target := int(endOfDeps) + len(depsToWrite)*8
			depsToWrite = append(depsToWrite, entry.Dependencies...)
			link.Offset = uint32(target - offsetFieldOffset)
		}
		binary.Write(buf, binary.LittleEndian, link)
	}

	// write Dependencies to be written
	for _, depLink := range depsToWrite {
		binary.Write(buf, binary.LittleEndian, depLink)
	}

	// write 8 nullbytes
	var nulls uint64 = 0
	binary.Write(buf, binary.LittleEndian, nulls)

	b := buf.Bytes()
	return &b
}

func ParseDependencies(b []byte) (*Dependencies, error) {
	s := parseDependencies{}
	s.IDToConn = make(map[uint64]DepLinks)
	reader := bytes.NewReader(b)
	err := binary.Read(reader, binary.LittleEndian, &s.Hdr)
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(s.Hdr.NumberOfIDs); i++ {
		var newID uint64
		binary.Read(reader, binary.LittleEndian, &newID)
		s.IDs = append(s.IDs, newID)
	}
	binary.Read(reader, binary.LittleEndian, &s.FileLength)
	curr, _ := reader.Seek(0, io.SeekCurrent)
	var x DepLinks
	s.OffsetAfterConss = curr + int64(s.Hdr.NumberOfIDs)*int64(binary.Size(x))
	for i := 0; i < int(s.Hdr.NumberOfIDs); i++ {
		var conn DepLinks
		binary.Read(reader, binary.LittleEndian, &conn)
		// adjust so that this offset can be used to index the resulting array of connections...
		if conn.Offset != 0 {
			curr, _ := reader.Seek(0, io.SeekCurrent)
			conn.Offset = uint32(curr) + conn.Offset - uint32(s.OffsetAfterConss) - 8
		}
		s.IDToConn[s.IDs[i]] = conn
		s.Conns = append(s.Conns, conn)
	}
	s.OffsetAfterConss, err = reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// parse the remainder of the file as IDs I guess?
	toParse := s.FileLength - uint32(s.Hdr.NumberOfIDs)*uint32(binary.Size(s.Conns[0]))
	for i := 0; i < int(toParse); i += 8 {
		var id uint64
		binary.Read(reader, binary.LittleEndian, &id)
		s.Deps = append(s.Deps, id)
	}
	return s.extractDependencies(), nil
}
