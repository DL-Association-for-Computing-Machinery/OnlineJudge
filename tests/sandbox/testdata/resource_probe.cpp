// Synthetic, bounded probes. Compile and run only in the dedicated sandbox.
#include <cerrno>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <linux/magic.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <sys/statvfs.h>
#include <sys/vfs.h>
#include <sys/wait.h>
#include <unistd.h>

using U = unsigned long long;
const char* dir = "/tmp/oj-resource-probe";

struct Snapshot {
    U bytes, available, inodes, free_inodes;
    bool tmpfs;
    bool read() {
        struct statvfs v{};
        struct statfs f{};
        if (statvfs("/tmp", &v) || statfs("/tmp", &f)) return false;
        bytes = U(v.f_blocks) * v.f_frsize;
        available = U(v.f_bavail) * v.f_frsize;
        inodes = v.f_files;
        free_inodes = v.f_ffree;
        tmpfs = f.f_type == TMPFS_MAGIC;
        return true;
    }
    void print() const {
        printf("{\"bytes\":%llu,\"available\":%llu,\"inodes\":%llu,"
               "\"free_inodes\":%llu,\"tmpfs\":%s}",
               bytes, available, inodes, free_inodes, tmpfs ? "true" : "false");
    }
};

int processes() {
    int pipefd[2];
    struct rlimit limit{};
    if (pipe(pipefd) || getrlimit(RLIMIT_NPROC, &limit)) return 90;
    pid_t children[32];
    int count = 0, error = 0, reaped = 0;
    // Only this parent forks. Children close the write end before blocking.
    for (; count < 32; ++count) {
        pid_t pid = fork();
        if (pid == 0) {
            close(pipefd[1]);
            char c;
            ssize_t n;
            do { n = read(pipefd[0], &c, 1); } while (n < 0 && errno == EINTR);
            _exit(n == 0 ? 0 : 91);
        }
        if (pid < 0) { error = errno; break; }
        children[count] = pid;
    }
    // Release and reap every successful child, including unexpected failures.
    close(pipefd[0]);
    close(pipefd[1]);
    for (int i = 0; i < count; ++i) {
        int status = 0;
        pid_t pid;
        do { pid = waitpid(children[i], &status, 0); } while (pid < 0 && errno == EINTR);
        if (pid == children[i] && WIFEXITED(status) && WEXITSTATUS(status) == 0) ++reaped;
    }
    pid_t again = fork();
    if (again == 0) _exit(0);
    int status = 0;
    pid_t waited = -1;
    if (again > 0) {
        do { waited = waitpid(again, &status, 0); } while (waited < 0 && errno == EINTR);
    }
    bool reused = again > 0 && waited == again && WIFEXITED(status) && WEXITSTATUS(status) == 0;
    printf("{\"children\":%d,\"error\":%d,\"eagain\":%s,\"reaped\":%d,"
           "\"reused\":%s,\"nproc_soft\":%llu}\n",
           count, error, error == EAGAIN ? "true" : "false", reaped,
           reused ? "true" : "false", U(limit.rlim_cur));
    return 0;
}

int inodes() {
    Snapshot before{}, full{}, reused{};
    if (!before.read() || mkdir(dir, 0700)) return 90;
    int count = 0, error = 0;
    char path[100];
    for (; count < 8192; ++count) {
        snprintf(path, sizeof(path), "%s/f-%d", dir, count);
        int fd = open(path, O_CREAT | O_EXCL | O_WRONLY, 0600);
        if (fd < 0) { error = errno; break; }
        if (close(fd)) return 91;
    }
    if (!full.read() || count == 0) return 92;
    snprintf(path, sizeof(path), "%s/f-%d", dir, count - 1);
    if (unlink(path)) return 93;
    int fd = open(path, O_CREAT | O_EXCL | O_WRONLY, 0600);
    if (fd < 0 || close(fd) || !reused.read()) return 94;
    printf("{\"before\":"); before.print();
    printf(",\"full\":"); full.print();
    printf(",\"reused\":"); reused.print();
    printf(",\"count\":%d,\"error\":%d,\"enospc\":%s}\n",
           count, error, error == ENOSPC ? "true" : "false");
    // Leave the exhausted filesystem for the executor's teardown, not cleanup.
    return 0;
}

int bytes(bool reuse) {
    Snapshot before{}, full{}, released{};
    struct rlimit limit{};
    if (!before.read() || getrlimit(RLIMIT_FSIZE, &limit) || mkdir(dir, 0700)) return 90;
    char buffer[65536];
    char path[100];
    memset(buffer, 0x5a, sizeof(buffer));
    U written = 0;
    int error = 0, count = 0;
    // Stay within the existing 1MiB per-file cap. No sparse allocation,
    // buffering, open-fd accumulation, or modification of any rlimit.
    for (int i = 0; i < 80; ++i) {
        snprintf(path, sizeof(path), "%s/f-%d", dir, i);
        int fd = open(path, O_CREAT | O_EXCL | O_WRONLY, 0600);
        if (fd < 0) { error = errno; break; }
        ++count;
        U file_bytes = 0;
        while (file_bytes < 1024 * 1024) {
            ssize_t n = write(fd, buffer, sizeof(buffer));
            if (n < 0) { error = errno; break; }
            if (n == 0) break;
            file_bytes += U(n);
            written += U(n);
        }
        if (close(fd)) return 91;
        if (error || file_bytes != 1024 * 1024) break;
    }
    if (!full.read()) return 92;
    if (reuse) {
        for (int i = 0; i < count; ++i) {
            snprintf(path, sizeof(path), "%s/f-%d", dir, i);
            if (unlink(path)) return 93;
        }
        if (!released.read()) return 93;
        snprintf(path, sizeof(path), "%s/reused", dir);
        int fd = open(path, O_CREAT | O_EXCL | O_WRONLY, 0600);
        if (fd < 0) return 94;
        ssize_t n = write(fd, buffer, 4096);
        if (close(fd) || n != 4096) return 95;
    }
    printf("{\"before\":"); before.print();
    printf(",\"full\":"); full.print();
    printf(",\"released\":"); released.print();
    printf(",\"written\":%llu,\"error\":%d,\"enospc\":%s,\"file_limit\":%llu,\"count\":%d}\n",
           written, error, error == ENOSPC ? "true" : "false", U(limit.rlim_cur), count);
    return 0;
}

int file_size() {
    struct rlimit limit{};
    if (getrlimit(RLIMIT_FSIZE, &limit) || mkdir(dir, 0700)) return 90;
    printf("{\"file_limit\":%llu}\n", U(limit.rlim_cur));
    if (fflush(stdout)) return 91;
    int fd = open("/tmp/oj-resource-probe/data", O_CREAT | O_EXCL | O_WRONLY, 0600);
    if (fd < 0) return 92;
    char buffer[65536];
    memset(buffer, 0x5a, sizeof(buffer));
    // The original profile must stop this finite 1MiB+64KiB write as SIGXFSZ.
    U written = 0;
    while (written < 17ULL * sizeof(buffer)) {
        ssize_t n = write(fd, buffer, sizeof(buffer));
        if (n <= 0) break;
        written += U(n);
    }
    close(fd);
    return 93;
}

int clean() {
    Snapshot s{};
    struct stat st{};
    int found = stat(dir, &st);
    int error = errno;
    if (!s.read() || found == 0 || error != ENOENT) return 90;
    s.print(); puts("");
    return 0;
}

int main() {
    char mode[32]{};
    if (scanf("%31s", mode) != 1) return 90;
    if (!strcmp(mode, "processes")) return processes();
    if (!strcmp(mode, "inodes")) return inodes();
    if (!strcmp(mode, "file-size")) return file_size();
    if (!strcmp(mode, "bytes-full")) return bytes(false);
    if (!strcmp(mode, "bytes-reuse")) return bytes(true);
    if (!strcmp(mode, "clean")) return clean();
    return 91;
}
