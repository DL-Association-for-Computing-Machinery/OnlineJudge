#include "testlib.h"

int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);
    if (inf.readInt(1, 2) == 1)
        quitp(0.5, "unsupported points result");
    quitf(_partially, "unsupported partial result");
}
