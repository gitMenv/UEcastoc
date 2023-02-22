#include <iostream>
#include "castoc_x64.h"

#include <string>
#include <vector>

using namespace std;


// Print help text on usage
void printHelp() {
    cout << "Usage: castoc.exe <feature> [args]" << endl;
    cout << "All args that are prepended with an asterisk are optional" << endl;
    cout << "Features:" << endl;
    cout << "  help: Print this message" << endl;
    cout << "  list [utocPath, *AES key]: lists all files that are packed in the .utoc/.ucas file" << endl;
    cout << "  unpackAll [utocPath, ucasPath, outputDir, *AES key]: unpack entire .utoc/.ucas files" << endl;
    cout << "  unpack [utocPath, ucasPath, outputDir, regex, *AES key]: unpack .utoc/.ucas files based on regex" << endl;
    cout << "  manifest [utocPath, ucasPath, outputManifest, *AES key]: creates Manifest file of this .utoc/.ucas file" << endl; 
    cout << "  pack [packDir, manifestPath, outputFile, compressionMethod, *AES key]: pack directory into .utoc/.ucas file" << endl;
    cout << endl;
    cout << "the pack command requires the manifest file, and it packs the input dir to the outputFile{.utoc, .ucas, .pak}; three files are created!" << endl;
    cout << "the following compression methods for packing are supported; {None, Zlib, Oodle}" << endl;
}

void help(vector<string> args) {
    printHelp();
}

void list(vector<string> args) {
    if (args.size() == 0){
        cout << "expecting at least one arg for list" << endl;
        printHelp();
        return;
    }
    int n;
    char** list;
    char* aeskey = NULL;
    const char *utocPath = args[0].c_str();

    if (args.size() > 1){
        const char *aes = args[1].c_str();
        aeskey = const_cast<char*>(aes);
    }
    list = listGameFiles(const_cast<char*>(utocPath), &n, aeskey);
    if(list == NULL){
        cout << getError() << endl;
        return;
    }
    for(int i = 0; i < n; i++){
        cout << list[i] << endl;
    }
    freeStringList(list, n);
}

void unpackAll(vector<string> args){
    //[utocPath, ucasPath, outputDir, *AES key]
    if (args.size() < 3){
        cout << "expecting at least three args for unpackAll" << endl;
        printHelp();
        return;
    }
    const char* utocPath = args[0].c_str();
    const char* ucasPath = args[1].c_str();
    const char* outputDir = args[2].c_str();

    char* aeskey = NULL;
    if (args.size() == 4) {
        const char* aes = args[3].c_str();
        aeskey = const_cast<char*>(aes);
    }
    int n = unpackAllGameFiles(const_cast<char*>(utocPath), 
        const_cast<char*>(ucasPath), 
        const_cast<char*>(outputDir), 
        aeskey);
    if(n < 0){
        cout << getError() << endl;
    } else {
        cout << "number of unpacked files:" << n << endl;
    }
}

void unpack(vector<string> args){
    //[utocPath, ucasPath, outputDir, regex, *AES key]
    if (args.size() < 4){
        cout << "expecting at least four args for unpack" << endl;
        printHelp();
        return;
    }
        const char* utocPath = args[0].c_str();
    const char* ucasPath = args[1].c_str();
    const char* outputDir = args[2].c_str();
    const char* regex = args[3].c_str();

    char* aeskey = NULL;
    if (args.size() == 5) {
        const char* aes = args[4].c_str();
        aeskey = const_cast<char*>(aes);
    }
    int n = unpackGameFiles(const_cast<char*>(utocPath), 
        const_cast<char*>(ucasPath), 
        const_cast<char*>(outputDir), 
        const_cast<char*>(regex),
        aeskey);
    if(n < 0){
        cout << getError() << endl;
    } else {
        cout << "number of unpacked files:" << n << endl;
    }
}

void manifest(vector<string> args){
    //[utocPath, ucasPath, outputManifest, aeskey]
    if(args.size() < 3) {
        cout << "expecting at least 3 arguments for creating a manifest file" << endl;
        printHelp();
        return;
    }
    const char* utocPath = args[0].c_str();
    const char* ucasPath = args[1].c_str();
    const char* outputManifest = args[2].c_str();
    char* aeskey = NULL;
    if (args.size() == 4) {
        const char* aes = args[3].c_str();
        aeskey = const_cast<char*>(aes);
    }
    int n = createManifestFile(const_cast<char*>(utocPath),
        const_cast<char*>(ucasPath),
        const_cast<char*>(outputManifest),
        aeskey);
    if (n < 0){
        cout << getError() << endl;
    }

}

void pack(vector<string> args){
    // [packDir, manifestPath, outputFile, compressionMethod, *AES key]
    if(args.size() < 4){
        cout << "expecting at least 4 arguments for packing" << endl;
        printHelp();
        return;
    }
    const char* packdir = args[0].c_str();
    const char* manifestPath = args[1].c_str();
    const char* outputfile = args[2].c_str();
    const char* compression = args[3].c_str();
    char* aeskey;
    if(args.size() == 5){
        const char* aes = args[4].c_str();
        aeskey = const_cast<char*>(aes);
    }
    int n = packGameFiles(const_cast<char*>(packdir),
        const_cast<char*>(manifestPath),
        const_cast<char*>(outputfile),
        const_cast<char*>(compression),
        aeskey);
    if(n < 0){
        cout << getError() << endl;
    }else{
        cout << "number of files packed:" << n << endl;
    }

}

int main(int argc, char** argv) {
    if (argc < 2) {
        cout << "Error: No feature specified" << endl;
        printHelp();
        return 1;
    }

    string feature = argv[1];
    vector<string> args;
    for (int i = 2; i < argc; i++) {
        args.push_back(argv[i]);
    }

    if (feature == "help") {
        help(args);
    } else if (feature == "list") {
        list(args);
    } else if(feature == "unpackAll"){
        unpackAll(args);
    } else if(feature == "unpack"){
        unpack(args);
    } else if(feature == "manifest"){
        manifest(args);
    } else if(feature == "pack") {
        pack(args);
    }else{
        cout << "Error: Invalid feature specified" << endl;
        printHelp();
        return 1;
    }

    return 0;
}

