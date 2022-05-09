# utoc File Format

This document shows the file format of utoc files, and they show their relation with the .ucas files for the Unreal Engine.
This has been reverse engineered based on the game files of Grounded, and the source code of the Unreal Engine.
That being said, let's dive right into it.

First the header:
For any extra data structure used in the header, an indent is made.
Everything in the file is Little Endian.
```
FILE HEADER, total bytes: 144
    byte {16}           - utoc MAGIC word (string: "-==--==--==--==-") 
    uint32 {4}          - Version, (2) (actually an 8-bit number, with three bytes reserved afterwards)
    uint32 {4}          - Header Size (144)
    uint32 {4}          - Entry Count
    uint32 {4}          - Count of Compressed Blocks 
    uint32 {4}          - Entry size of Compressed Block (12)
    uint32 {4}          - Count Compression method names 
    uint32 {4}          - Compression method name length (32)
    uint32 {4}          - Compression block size (0x10000)
    uint32 {4}          - Directory index Size
    uint32 {4}          - Partition Count (0) // default value is 0
    uint64 {8}          - Container ID (FIoContainerID)
    FGUID {16}          - Guid Encryption Key
        uint32 {4}      - A
        uint32 {4}      - B
        uint32 {4}      - C
        uint32 {4}      - D
    uint8 {1}           - Container flags
    byte {63}           - reserved, padding, and "partition size" is among them as well, but seems unused...
```

Based on the header, there are several number of data structures parsed.
The way in which these data structures are parsed and how often they occur are listed here, in the form: "(data structure): {name of variable}"
The individual data structures can be found below.
```
(CHUNK_ID)            : {Entry Count}
(OFFSET_AND_LENGTHS)  : {Entry Count}
(COMPRESSION_BLOCK)   : {Count of Compressed Blocks}
(byte {32})           : {Count Compression method names} // the 32 is derived from the compression method name length
(DIRECTORY_INDEX)     : {1} // number of bytes in data structure: Directory index Size
(CHUNK_META)          : {Entry Count}
```
Note: while there are Count Compression method names entries in some kind of array with the compression methods, this array should start at 1, as index 0 means "No compression method".

After the chunk meta has been read, there should not be any bytes left to parse from the .utoc file.

________
Data structures:

```
CHUNK_ID, total bytes: 12
    uint64 {8}          - ID
    uint16 {2}          - Index // seems to be always 0
    uint8  {1}          - Padding // making it 0x10 aligned
    uint8  {1}          - Type
```
These identify one chunk. 
As far as I know, the ID is invalid if the value is 0.
Otherwise it seems to be random.
I am not sure what the Index means.
The Type field corresponds to the EIoChunkType of the unreal engine.
The values I've seen are 2, 3 and 10.

Value 2 means "BulkData" and is used to represent .uasset files.
Value 3 means "OptionalBulkData" and is used to represent .ubulk files.
The very first chunkID always has value 10, which means "PackageStoreEntry", but I have no clue what this represents.
The Unreal Engine code isn't too clear about it.
I suspect that this has something to do with the FPackageStoreEntry struct.

To investigate what it could mean, I set everything of those sections to 0 and checked with the UnrealPak Viewer tool (https://github.com/jashking/UnrealPakViewer).
In the one with everything set to zero, it has "dependency counts" at 0, and the class entries are set to "uexp" or "ubulk".
For the other version, the one with data, it has various "dependency counts", and the class entries are set to different things, such as "Texture2D", "StaticMesh", "Skeleton", "SoundWave" etc.
Furthermore, the "ExportObjects" field in the empty one has counts 0, while the one with data does not.


Update: I have found a few more things about this. 
If the UTOC header version is 2, then the first entry in the list of chunk IDs has type 10, which is not "mapped" to a file.
If the UTOC header version is 3, then this is the last entry.
The compression blocks that the corresponding offset and lengths entry corresponds to, are compressed (or not), and when decompressed, they form a file with some special file format.
I have dubbed this file format as "depsFile", and its specification can be found below.



```
OFFSET_AND_LENGTHS, total bytes: 10
    byte {5}            - Offset
    byte {5}            - Length
```
Read as a uint64, with only the lower 5 bytes set, the others should be 0.
There are just as many of these entries as Chunk IDs and file entries.
The offset and lengths actually show the data of the individual files that are compressed in the .ucas file.
Note that all data must be decompressed first.

**NOTE:** The length of the offlengths show the length of the decompressed file!
The offset follows directly after the next length, but it's aligned to 0x10000. 
Therefore, there may be large gaps between them.
This is done for indexing in the array of chunks.

```
COMPRESSION_BLOCK, total bytes: 12
    byte {5}            - Offset
    byte {3}            - Compressed block size
    byte {3}            - Uncompressed block size
    uint8 {1}           - Compression method
``` 
Read offset as uint64, with only lower 5 bytes set.
The others should be read as uint32, with only the lower 3 bytes set.
The compression method is interpreted as an index in the list of compression method names.

The offset is an offset in the .ucas field, and it is always 16-aligned.
The compression block size is the length of the data in the .ucas field.
If the compression block size equals the uncompressed block size, it is obviously not compressed.
If it is, the compression method is an index in the list of compression method entries.
One note: in case of 0, this is always "None".

As the offset values are always 16-aligned, there are bogus values in between the parts that are unallocated.
I don't know the significance of these values, but I think they are just random.


```
DIRECTORY_INDEX (total bytes is variable)
    uint32 {4}          - Mount point string length (includes nullbyte)
    byte {X}            - Mount point string
    uint32 {4}          - Directory Index array Entry Count (DIEC)
    DIR_INDEX_ENTRY {DIEC * 16} // an array of DIR_INDEX_ENTRY
        uint32 {4}      - Directory Name index
        uint32 {4}      - First child entry index
        uint32 {4}      - First sibling entry index
        uint32 {4}      - First file entry index
    uint32 {4}          - File Index array Entry Count (FIEC)
    FILE_INDEX_ENTRY {FIEC * 12} // an array of FILE_INDEX_ENTRY
        uint32 {4}      - File name index
        uint32 {4}      - Next file entry index
        uint32 {4}      - UserData (index, I guess)
    uint32 {4}          - number of strings of the string array 
    FSTRING {number_of_strings * (4 + X)} // an array of strings, referred to as StringTable
        uint32 {4}      - string length, nullbyte included
        byte {X}        - actual string
```
The directory index entries and the file index entries have a "name" index, which is used to refer to a name in the StringTable.
The child and sibling indices for the Directory entries point to a different Directory entry in the same array.
The file entry indices point to the file entry in the FILE_INDEX_ENTRY array.

```
CHUNK_META, total bytes: 33
    byte {32}        - Hash of chunk (This is standard SHA1 hash)
    uint8 {1}        - Flags
```
Possible flags are: NoneMetaFlag (0), CompressedMetaFlag (1), MemoryMappedMetaFlag (2)


____
### depsFile Format
This still belongs to the .utoc file format, but it's encoded in the .ucas file.
In the .ucas file, either the first large chunk of data (in header version 2), or the last chunk of data (in version 3) is specified as follows:

```
depsFile HEADER, total bytes: 32
    uint64 {8}      -   Chunk ID of this "file"
    uint64 {8}      -   Number of IDs (nID)
    uint32 {4}      -   sizeof(chunkID) (always 8)
    uint32 {4}      -   as of now, an unknown value
    byte   {4}      -   zero bytes PADDING
    uint32 {4}      -   Number of IDs (again, but now uint32?)
```
The header is followed by an ORDERED list of Chunk IDs, of size _nID_. 
The list is unique, and it contains all of the IDs that are present in the .utoc file.
I suspect that this is done for performance, for quick searching.

The list is immediately followed by a uint32 value describing the size in bytes of the rest of the file.
It excludes the trailling (8) zerobytes.
The "rest of the file" seems to consist of some kind of data of which I'm not sure what it means yet, followed by yet another list of chunk IDs.

____
# UAsset File Format
After unpacking the .ucas file using the .utoc file, I found out that the .uasset files that were created differ quite from the original .uasset files.
They used to be different after unpacking with the .pak files.
Therefore, I decided to figure out the (new) .uasset file format after unpacking.

A notable change is that when unpacking, the unreal signature (C1 83 2A 9E) is omitted.
The version number is omitted as well. 
I think this is done to preserve space, as these are probably constant among all .uasset files in the .ucas file.

In addition to this, there is no split between .uasset and .uexp anymore; everything is concatenated within one .uasset file.
This is also the intended way in which .uasset files are stored, I believe.
It's just useful to split the two in some cases.

The uasset header is divided into two parts; a constant file header (of size 64), and a variable sized part.
After the complete .uasset header, the contents of the would be .uexp file starts.
```
UASSET HEADER, total bytes: 64
    uint64 {8}      - no idea what this is, but it is repeated twice
    uint64 {8}      - same as above
    uint32 {4}      - "Package Flags" (00 00 00 80) no idea what this does, but it's constant everywhere
    uint32 {4}      - "total header size"?
    uint32 {4}      - Names Directory Offset (points to nullbyte, so do + 1) (always 0x40, length of header)
    uint32 {4}      - NamesDirectory Length (in bytes)
    uint32 {4}      - absolute pointer to the start of something else.
    uint32 {4}      - some other value?
    uint32 {4}      - offset in .uasset file to the "EXTRA INFO" part
    uint32 {4} // 48 - 
    byte {32}

```
```
EXTRA INFO
    uint64 {8}      - "Total header size", it's not actually total header size though... It's something else.
    uint64 {8}      - total file size of the .uexp part of the file
    uint64 {8}      - offset of the current file name in the list of strings!

```

Looking at table_crafting_intermediatematerials.uasset file...


```
NAMES DIRECTORY
for each name
    uint8 {1}       - Name length (EXCLUDING null terminator)
    byte  {X}       - Name
    byte  {1}       - null terminator
```

```
IMPORT DIRECTORY


```