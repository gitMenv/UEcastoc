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
I am not sure what the Index means, as this value is always 0.
Maybe this is relevant if the files are too large to be described by one chunk.
In that case you can have multiple chunks describing a file, possibly with multiple indices, but this is all speculation.
The Type field corresponds to the EIoChunkType of the unreal engine.
The values I've seen are 2, 3 and 10.

Value 2 means "BulkData" and is used to represent .uasset files.
Value 3 means "OptionalBulkData" and is used to represent .ubulk files.
The .uasset files all have unique chunk IDs, whereas the .ubulk files have an ID that was already taken by a .uasset file.
The very last chunkID always has value 10, which means "PackageStoreEntry", but I have no clue what this represents.
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
    byte {32}        - Hash of chunk (This is standard SHA1 hash of 20 bytes, with 12 bytes of padding)
    uint8 {1}        - Flags
```
Possible flags are: NoneMetaFlag (0), CompressedMetaFlag (1), MemoryMappedMetaFlag (2)
However, in the Grounded utoc file, the flag value is always 1.


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
There's just one Chunk ID missing, and that's the thing with the chunk type being 10 (so the one that contains this "depsFile Format").

The list is immediately followed by a uint32 value describing the size in bytes of the rest of the file.
It excludes the trailling (8) zerobytes.

The "rest of the file" consists of the following very interesting information.
First, there are blocks of 32 bytes, with data that links .uasset files to other files.
These are the connections that are made between dependencies, and they show how many exports each entry has.
For every single ID that was provided in the previous part with the chunk IDs, there is a corresponding entry with this structure;

```
depsFile DEPENDENCIES, total bytes: 32
    uint64 {8}      - File size (uncompressed)
    uint32 {4}      - Number of Export Objects
    uint32 {4}      - A number either 1, 2 or 3. Not sure what this means

    uint64 {8}      - Index of some sort; unique and starting at 0, but skipping some numbers
    uint32 {4}      - Number of Dependency Packages
    uint32 {4}      - Offset to dependency packages
```
The list of these dependencies is followed by a long list of chunk IDs, which are in fact the dependencies.
The offset in a dependency entry points to the start of such chunk ID, but these offsets are a bit tricky.
This offset is taken from the specific offset entry, before reading the entry, so all offsets jump over the whole list.
The number of dependency packages tells how far one must read!
I think this could have been done a bit more efficiently, but unfortunately I didn't design this file format.

The number of IDs in the dependency area can be calculated.
The length of the "rest of the file" is known, which consists of a number of dependency-entries and the IDs.
We know how many dependency-entries there are, so we can calculate how many IDs there are.
The list of IDs is followed by 8 nullbytes, which dictates the end of the file.

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

Note: most of this information is reverse-engineerd by looking at the information in the .ucas viewer linked before.
```
UASSET HEADER, total bytes: 64
    uint64 {8}      - Name in FMappedName format.  See https://docs.unrealengine.com/5.0/en-US/API/Runtime/Core/Serialization/FMappedName/
    uint64 {8}      - SourceName in FMappedName format
    uint32 {4}      - "Package Flags" Generally constant at (00 00 00 80).  See https://docs.unrealengine.com/5.1/en-US/API/Runtime/CoreUObject/UObject/EPackageFlags/ 
    uint32 {4}      - CookedHeaderSize - This is the total header size, if it were an old .uasset file, so it deviates
    int32 {4}      - Name Map Offset
    int32 {4}      - Name Map Size (in bytes)
    int32 {4}      - Name Map Hashes offset
    int32 {4}      - Name Map Hashes Size (in bytes)
    int32 {4}      - Import Map Offset
    int32 {4}      - Export Map offset
    int32 {4}      - Export Bundles Offset
    int32 {4}      - Graph Data Offset / Dependency Packages offset (also duplicated in dependency file in .ucas)
    int32 {4}      - Graph Data Size / Dependency Package Size (in bytes)
    int32 {4}      - Padding
```
The header indicates a lot of offsets of all the parts in the .uasset file.
The remainder of this section highlights each of these parts in order.
Sometimes, the header lists the length of a few parts, but I'm not sure why it does this, as all lengths can be inferred from the other offsets, except maybe the list of strings.

Directly after the file header, there is one nullbyte, after which the name directory index is shown.
```
NAMES DIRECTORY
for each name
    uint8 {1}       - Name length (EXCLUDING null terminator)
    byte  {X}       - Name
    byte  {1}       - null terminator
```
Due to the strings in the Names Directory being of different sizes, it is possible that the byte count is not aligned anymore.
After the Names Directory, there are nullbytes to make it aligned to 8 bytes.
This is probably done for performance reasons.


### String Hashes
The names directory is followed by a list 64-bit hashes, representing the hashes of the strings.
The very first value is the "AlgorithmID" of the hash.
This value seems to be always `00 00 64 C1 00 00 00 00`, and this corresponds to the code in the [Unreal Engine](https://github.com/EpicGames/UnrealEngine/blob/46544fa5e0aa9e6740c19b44b0628b72e7bbd5ce/Engine/Source/Runtime/Core/Private/UObject/UnrealNames.cpp) (line 489)!
The hashing algorithm is the CityHash64 algorithm, and it works on strings that were made to be lowercase.
So to get the value, make the string lowercase and hash it with the CityHash64 algorithm.

### Import Objects
The list of string hashes is followed by a list of Import Object IDs.
This is not the same as the file IDs, and I'm not sure what these values represent.
Each value is 8 bytes, and if the import name is "None", the value is `0xFFFFFFFFFFFFFFFF`, or -1.
Not sure what to do with this, but the number of entries correspond to the number of import objects in the ucas file viewer.

In Helios' tool, the Import Objects correspond to the Linked Classes.


### Export Objects
The next area of the file, are Export Objects.
These are stored in a specific file structure, shown below.

```
EXPORT OBJECTS
    uint64 {8}      - Serial Offset 
    uint64 {8}      - Serial Size
    uint64 {8}      - Object Name Offset
    uint64 {8}      - Class Name Offset
    byte {40}       - this is still unidentified at this moment.
```
The fields that were used are mainly copied from the .ucas file viewer. 
The unknown 40 bytes are still unidentified, but the number of bytes seem to be correct.
In any case, the offsets that are described is an integer describing the offset in the name directory, or the list of strings.

The part after the .uasset file header is built up from different ExportObjects.
When there are multiple ExportObjects, they are consecutive, which the SerialOffset and sizes illustrate.
For the very first ExportObject, the Serial Offset field is the exact same value as the "total header size" in the header of the file.
Now in this new file format, this doesn't really hold anymore, as it differs from the regular file.
However, the serial offset states where the .uexp portion of the file starts.
The Serial Size also has great significance, as this is the length of the .uexp portion of the file.

The object name offset and the class name offset probably have some use as well, but these probably refer to the old file format.
Therefore, the offset is off and I'm not sure how to correct it right now.
Changing files is probably mostly impacting the serial size and the serial offset, so I'll keep this as a mystery for now.

### ExportObject metadata?
The export objects are followed by a strange data structure.
It first starts with a 64-bit number, which seems to have no relation to the rest.
Afterwards, there is an enumeration of some kind of metadata to the export objects.
The metadata consists of four 32-bit numbers, and there are exactly as many metadata entries as export objects.
This is still a mystery, but I don't think I'll touch ExportObjects anyway.


### Dependency packages
After all export objects information, there is a list of dependency packages.
It starts with a uint32 stating how many dependency entries there are.
After this, the following dependencies are shown;
```
DEPENDENCY PACKAGE
    uint64 {8}      - ID
    uint32 {4}      - Number of Entries (?) // Not quite sure, it's often just 1, sometimes 2 and 3.
    int32 {4}       - is present*
    uint32 {4}      - mostly one // not sure what it is, but almost always 1.
    
    if (number of entries > 1){
        uint32 {4 * (2 * (numberOfEntries - 1))}**
    }
```
`*` If the value of "is present" is 0, then the ID listed is included in the current .ucas file.
If this value is -1, it's not included in the current .ucas file.

`**` If the number of entries is 2 or 3, there are more numbers defined.
For every number larger than 1, there are two additional uint32 numbers in a dependency entry.

It's quite clear that I am still not sure what all of the fields mean and how they relate to each other.
However, after the list of the dependencies, the entire former .uexp file starts!

### End of the header
This concludes the .uasset header part of the file.
There are quite some values that are still unknown to me.
However, most of these values will probably not change based on the created mods.
I can only hope that I know enough to do the most interesting stuff with!
