#include "testlib.h"
#include <cstdio>

int main(int argc, char** argv) {
    registerTestlibCmd(argc, argv);
    for (int i = 0; i < 8000; ++i) std::fputc('x', stderr);
    quitf(_ok, "diagnostic control");
}
