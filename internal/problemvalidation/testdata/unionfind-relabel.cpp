#include <iostream>
#include <numeric>
#include <vector>

int main() {
    std::ios::sync_with_stdio(false);
    std::cin.tie(nullptr);
    int n, q;
    if (!(std::cin >> n >> q)) return 1;
    std::vector<int> label(n);
    std::iota(label.begin(), label.end(), 0);
    for (int i = 0; i < q; ++i) {
        int op, a, b;
        if (!(std::cin >> op >> a >> b)) return 1;
        const int from = label[b], to = label[a];
        if (op == 1) {
            std::cout << (from == to) << '\n';
        } else if (from != to) {
            for (int &c : label) {
                if (c == from) c = to;
            }
        }
    }
}
