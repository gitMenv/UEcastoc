package main

import (
	"fmt"
)

func main() {
	path := "./files/Maine"
	hdr, err := ParseUTocHeader(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("hdr size must be 144: %+v\n", hdr.Header.HeaderSize)

	// unpacking a .ucas/.utoc file to the requested path.
	// a blacklist can be included to reduce much time for things you might not need

	// hdr.UnpackUCAS("unpacked", []string{"./unpacked/Engine/", "./unpacked/Maine/Content/UI/", "./unpacked/Maine/Content/ThirdParty/"})

	// packing a file back to .ucas/.utoc file format.
	// this does kind of work, but it misses the crucual part where userData would be 0. I don't have any idea what this means yet.

	// PackToUCAS("unpacked")

}
