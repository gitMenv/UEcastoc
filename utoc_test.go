package uecastoc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tenfyzhong/cityhash"
)

// just prints with a level of indentation; useful for listing the files and dirs
func printDirectoryStructure(d *Directory, level int) {
	indent := strings.Repeat(" ", level)
	for _, v := range d.ChildDirs {
		fmt.Println(indent, v.Name)
		printDirectoryStructure(v, level+4)
	}
	for _, v := range d.Files {
		fmt.Println(indent, v.Name)
	}
}

// pack a director to a .ucas/.utoc file combination.
// The packing procedure has been validated by using the ucasviewer.
// It does NOT work for the entire unpacked file structure, but not sure why not,
// as it does work for multiple files in a multi-level file structure.
func TestPackDirectory(t *testing.T) {
	PackDirectory("../Game/")
}

func TestDirectoryStructure(t *testing.T) {
	path := "C:/Program Files (x86)/Steam/steamapps/common/Grounded/Maine/Content/Paks/Maine-WindowsNoEditor"
	ct, _ := ParseUTocFile(path)
	printDirectoryStructure(ct.Root, 2)
}

// TestParseUTocFile parses the utoc/ucas files.
// If you uncomment the call to UnpackDirectory, it unpacks (and uncompresses) the
// root directory and its subdirectories of the file to the desired folder.
func TestParseUTocFile(t *testing.T) {
	// path to the utoc/ucas files...
	path := "C:/Program Files (x86)/Steam/steamapps/common/Grounded/Maine/Content/Paks/Maine-WindowsNoEditor"
	ct, err := ParseUTocFile(path)
	if err != nil {
		fmt.Println("err:", err)
	}
	fmt.Println(ct.ContainerID)

	// f, _ := os.Open(path + ".ucas")
	// ct.Root.UnpackDirectory(f, "./unpacked/")

	// the ParseUTocFile also saves some of the parsed data that must be used by the mods
	// I am not 100% if all of this data must be used or only few.
	v, err := loadParsedData()
	if err != nil {
		fmt.Println("err:", err.Error())
	} else {
		fmt.Println(v.Deps.ThisPackageID)
	}
}

// TestUASSET simply tests the parsing of the "new" uasset file format.
// This is far from done and still requires a lot of work.
func TestUASSET(t *testing.T) {
	// in this case, I was simply looking at the BP_SurvivalPlayerCharacter uasset file
	path := "C:/Users/Menno/Desktop/Modding/Go/unpacked3/Maine/Content/Blueprints/Items/CraftingTables/Table_Crafting_Equipment.uasset"
	// path := "C:/Users/Menno/Desktop/Modding/Go/unpacked3/Maine/Content/Blueprints/Items/Table_AllItems.uasset"

	src, err := ParseUAssetFile(path)
	// fmt.Println("there are the following \"ExportObjects\"")
	// for i, v := range src.ExportObjects {
	// 	fmt.Println("block", i)
	// 	fmt.Println("	name number:", v.ObjectNameOffset)
	// 	fmt.Printf("	range [%d - %d]\n", v.SerialOffset, v.SerialOffset+v.SerialSize)
	// }
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	_ = src
}

func TestCityhash(t *testing.T) {
	// just testing the cityhash
	value := []byte("datatable")
	hash := cityhash.CityHash64(value)
	fmt.Println("hash  :", hash)
	fmt.Println("should:", 8206203013534831085)
	fmt.Printf("hex       : 0x%X\n", hash)
	fmt.Printf("should HEX: 0x%X\n", 8206203013534831085)
}

// webWoven voor intermediateMaterials: 1393408675175398950
// webWoven voor table_craftingTools:   1393408675175398950
// 13 56 61 ce b6 c6 fe 00
