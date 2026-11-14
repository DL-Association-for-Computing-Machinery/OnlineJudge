#include "testlib.h"

int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);
    const int count = inf.readInt(1, 64);
    int low = 1000, high = -1000;
    for (int i = 0; i < count; ++i) {
        const int value = inf.readInt(-1000, 1000);
        low = std::min(low, value);
        high = std::max(high, value);
    }
    const int expected = ans.readInt(0, 2000);
    if (expected != high - low || !inf.seekEof() || !ans.seekEof())
        quitf(_fail, "invalid frozen range");
    int actual = ouf.readInt(-2000, 2000);
    if (!ouf.seekEof())
        actual += ouf.readInt(-2000, 2000);
    if (actual != expected)
        quitf(_wa, "incorrect range");
    quitf(_ok, "correct range");
}
