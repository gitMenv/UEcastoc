## UECastoc Design for DLL

I have started this project over a year ago, without knowing whether it would ever work.
Just jumping in and reverse engineering the file format has proved to be effective.
However, the resulting code is currently only really usable within Go only.
For most modding tools, it would be nice if you can create a user interface to go with it.
Go isn't meant for creating user interfaces, and not many people in the modding scene do anything with Go.
Therefore, I've decided to make this a DLL for Windows, so that you can still use the code that I wrote, but independent of Go.

The focus of this DLL is to create packing/unpacking support for Unreal Engine games with an API that is usable.
This API is documented below

## Would you like to support me?
You can donate using this [link](https://www.paypal.com/donate/?hosted_button_id=VLFVGJ749MQCC).

## Core Features of DLL

This section lists the core features that this DLL will support.
I do this by showing what the API functions should look like.

### Error Handling
Error handling is unique in Go.
There's even a specific type for it!
In C, error handling is often done by returning -1 or NULL if something went wrong.
If you need more information, it is possible to display a message using the _strerror_ function.

The displayed message originates from the _errno_ value.
From a DLL written in Go, this is not easy to set, though.
Therefore, I provided a function that simply returns the most recent error as a string.

```c
char *getError();
```

This can always be called from your program.
Whenever this function is called, do free the pointer that is provided!

### Listing Game Files
The .utoc game file indicates which files are included in the .ucas file.
This is a long list of game files which is structured in directories.
The DLL provides the following function for this.

```c
char **listGameFiles(char *utocFileName, int *n, char *AESKey);
```

The function requires the name of the .utoc file, as this file contains the list of game files.
You must also pass in a pointer to an integer _n_, which will contain the number of strings in the list when it returns.
Optionally, you can provide an AES key for utoc files that are encrypted with AES.
While this DLL is quite nice, it can not break AES encryption (yet).
Do provide the key as a hex string, otherwise simply pass NULL.

If the value of _n_ is -1 after calling, no strings were returned, and the char ** should be NULL.
Otherwise, the char ** contains _n_ strings, where each string is the entire path to a file.

The file is read and closed by the DLL, but the list must be freed by the caller.
This can also be done with the freeStringList function that comes with the DLL.
Just provide the pointer to the string list and the length of the list.
Do provide an accurate length, as otherwise memory leaks or double frees may occur.
```c
void freeStringList(char **stringlist, int n);
```
It goes without saying, but the pointer should not be used afterwards.


### Create Manifest File
A Manifest file is required to build game files into a mod file.
The Manifest file contains a list of filenames with their Chunk IDs, with some other data that is needed to build a mod for the game later.
```c
int createManifestFile(char *utocFileName, char *ucasFileName, char *outputFileName);
```
This function returns 0 upon success and -1 upon error. 
The error message can be retrieved using the getError function.


### Unpack all Game Files
Unpacking the game files require the .utoc file and the .ucas file.
All files get unpacked in the path with the provided directory.
The directory name should end with a slash, and the directory should exist.
An AES key is required for encrypted .utoc/.ucas files.
Pass NULL if it is not encrypted.
**WARNING: Depending on the .utoc and .ucas files, this can take a long time and it could fill your disk.**

```c
int unpackAllGameFiles(char *utocFileName, char *ucasFileName, char *outputDirectory, char *AESKey);
```
This function returns -1 upon error.
Any other value indicates the number of files that were extracted.
The error message can be retrieved using the getError function.


### Unpack a Selection of Game Files
Unpacking can take a LOT of time.
This function allows one to unpack only the files that matches the provided regular expression.
The regular expression syntax is the one that is default in the Golang language.
https://regex101.com/ has a golang option for regexes, so I assume this is special.
It is otherwise the same as the unpackAllGameFiles function.

```c
int unpackGameFiles(char *utocFileName, char *ucasFileName, char *outputDirectory, char *regex, char *AESKey);
```

### Packing Game Files
Packing the game files require the manifest file that you build using the function meant for it.
This function takes the game directory that you are packing, which should follow the same file structure as how it was unpacked.
The outFile is simply a path to a (new) filename, without any extension. 
The filename is used to create the .utoc, .ucas and .pak files in the path that you specify.

Three compression methods are currently known; "None", "Zlib", "Oodle" or "lz4".
None is the default, so when NULL is passed, it will not be compressed.
Any other compression method will return errors; the names are not case-sensitive.
If you wish to encrypt the created files, you could provide an AES key, but I am pretty sure Unreal Engine won't be able to decrypt your files.
I just added this encryption "feature" for experimentation.

```c
int packGameFiles(char *dirPath, char *manifestPath, char *outFile, char *compressionMethod, char *AESKey);
```
The function returns -1 in case of error. 
Otherwise, it returns the number of files that were packed into the .utoc/.ucas files that were created.

# Building the DLL yourself!

Building a DLL from Go on Windows is done as follows
```go
go build -o castoc_x64.dll -buildmode=c-shared -ldflags "-s -w" .
```
This creates a DLL file and a header file. 
Both must be placed in the directory of the C/C++ program you would like to build.
The resulting program can then be used without the header file, but it does require the DLL file!

Compiling a C++ program while making use of the DLL file is easy;
```cpp
g++ main.cpp castoc_x64.dll -o main.exe
```