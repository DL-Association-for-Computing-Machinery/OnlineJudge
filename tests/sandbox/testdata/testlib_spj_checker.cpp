#include "testlib.h"

int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);
    const int target = inf.readInt(-100, 100);
    const int expected = ans.readInt(-100, 100);
    if (target != expected)
        quitf(_fail, "fixture input and answer disagree");

    // The fixture permits either order of the two bounded summands.
    const int x = ouf.readInt(-100, 100);
    const int y = ouf.readInt(-100, 100);
    if (x + y != expected)
        quitf(_wa, "incorrect sum");
    quitf(_ok, "correct sum");
}
