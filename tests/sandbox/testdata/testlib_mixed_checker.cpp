#include "testlib.h"
#include <cmath>

int main(int argc, char** argv) {
    registerTestlibCmd(argc, argv);
    const int id = ans.readInt();
    const double value = ans.readDouble();
    if (inf.readInt() != id) quitf(_fail, "fixture input disagrees");
    if (ouf.readInt() != id) quitf(_wa, "wrong integer");
    if (std::abs(ouf.readDouble() - value) > 0.125) quitf(_wa, "outside tolerance");
    quitf(_ok, "strict integer and tolerant real");
}
