package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	castoc "github.com/gitMenv/UEcastoc"
)

type helpMessage struct {
	command     string
	explanation string
}

var (
	helpMessages []helpMessage = []helpMessage{
		{"h", "displays this help message"},
		{"ls", "<filepath> lists the files packed inside a .ucas file specified by the <filepath>. Works best when writing to a file"},
		{"u", "<filepath> <subdirectory*> unpacks the subdirectory (or root if none provided) of the .ucas file and exports it in the current directory."},
		{"p", "<dirpath> packs the specified directory into a .ucas/.utoc file; NOTE: requires the execution of either the -ls or -u command with a valid .ucas file first!"},
	}
)

func printHelp() {
	fmt.Println("One command can be used at a time. Pass these as arguments to the command line")
	fmt.Println("All commands/arguments indicated with an asterisk (*) are optional.")
	fmt.Println("Beware; unpacking or packing huge files may take a LONG time (up to 15 minutes)")
	fmt.Println("This tool can be used as follows")
	for _, msg := range helpMessages {
		fmt.Printf("    -%s\t\t\t- %s\n", msg.command, msg.explanation)
	}
	fmt.Println()
}

// just prints with a level of indentation; useful for listing the files and dirs
func printDirectoryStructure(d *castoc.Directory, level int) {
	indent := strings.Repeat(" ", level)
	for _, v := range d.ChildDirs {
		fmt.Println(indent, v.Name)
		printDirectoryStructure(v, level+4)
	}
	for _, v := range d.Files {
		fmt.Println(indent, v.Name)
	}
}

func listDirectories(path string) {
	// check if file exists
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		fmt.Println("error: the filepath provided is not a path to a file")
		return
	}
	ct, err := castoc.ParseUTocFile(path)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Printf("The file contents of %s\n", path)
	printDirectoryStructure(ct.Root, 2)
}

func unpackDirectory(ucaspath string, dirpath string) {
	if _, err := os.Stat(ucaspath); errors.Is(err, os.ErrNotExist) {
		fmt.Println("error: the filepath provided is not a path to a file")
		return
	}
	ct, err := castoc.ParseUTocFile(ucaspath)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	if dirpath == "" {
		f, _ := os.Open(ucaspath)
		fmt.Printf("unpacking the root directory into ./unpacked/\n")
		ct.Root.UnpackDirectory(f, "./unpacked/")
		f.Close()
	} else {
		dirList := strings.Split(dirpath, "/")
		if strings.Contains(dirpath, "\\") {
			dirList = strings.Split(dirpath, "\\")
		}
		currDir := ct.Root
		for i := 0; i < len(dirList); i++ {
			for _, child := range currDir.ChildDirs {
				if child.Name == dirList[i] {
					currDir = child
					break
				}
			}
		}
		f, _ := os.Open(ucaspath)
		fmt.Printf("unpacking the directory %s into ./unpacked/\n", currDir.Name)
		currDir.UnpackDirectory(f, "./unpacked/")
		f.Close()
	}
}

func main() {
	fmt.Println("Welcome to this simple packing/unpacking tool for ucas and utoc files.")
	fmt.Println("This project is by no means finished, but this showcases the things I've reverse engineered so far.")
	fmt.Println("The tool you're currently using uses the package that is hosted at github.com/gitMenv/UEcastoc")
	fmt.Println()
	if len(os.Args) == 1 {
		printHelp()
		return
	}

	switch os.Args[1][1:] {
	case "h":
		printHelp()
	case "u":
		if len(os.Args) <= 2 {
			fmt.Println("This command (u) requires an additional argument")
		}
		dirpath := ""
		if len(os.Args) > 2 {
			dirpath = os.Args[3]
		}
		unpackDirectory(os.Args[2], dirpath)
	case "p":
		if len(os.Args) <= 2 {
			fmt.Println("This command (p) requires an additional argument")
		}
		dirpath := os.Args[2]
		fmt.Println("packing directory:", dirpath, "into Packed_P.ucas and Packed_P.utoc")
		castoc.PackDirectory(dirpath)

	case "ls":
		if len(os.Args) <= 2 {
			fmt.Println("This command (ls) requires an additional argument")
			return
		}
		listDirectories(os.Args[2])
	}

}
