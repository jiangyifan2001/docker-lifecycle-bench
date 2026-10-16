#define _GNU_SOURCE

#include <errno.h>
#include <stdint.h>
#include <stdlib.h>
#include <time.h>
#include <unistd.h>

enum { FRAME_SIZE = 24 };

static void put_u32_be(unsigned char *dst, uint32_t value) {
    dst[0] = (unsigned char)(value >> 24);
    dst[1] = (unsigned char)(value >> 16);
    dst[2] = (unsigned char)(value >> 8);
    dst[3] = (unsigned char)value;
}

static void put_u64_be(unsigned char *dst, uint64_t value) {
    dst[0] = (unsigned char)(value >> 56);
    dst[1] = (unsigned char)(value >> 48);
    dst[2] = (unsigned char)(value >> 40);
    dst[3] = (unsigned char)(value >> 32);
    dst[4] = (unsigned char)(value >> 24);
    dst[5] = (unsigned char)(value >> 16);
    dst[6] = (unsigned char)(value >> 8);
    dst[7] = (unsigned char)value;
}

static int write_all(int fd, const unsigned char *buf, size_t size) {
    while (size > 0) {
        ssize_t written = write(fd, buf, size);
        if (written < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        buf += written;
        size -= (size_t)written;
    }
    return 0;
}

int main(void) {
    struct timespec timestamp;
    unsigned char frame[FRAME_SIZE] = {
        'D', 'L', 'B', 'E', 'N', 'C', 'H', '1',
        1, 0, 0, 0,
        0, 0, 0, 0, 0, 0, 0, 0,
        0, 0, 0, 0,
    };

    if (clock_gettime(CLOCK_MONOTONIC_RAW, &timestamp) != 0) {
        return 70;
    }

    uint64_t monotonic_ns =
        (uint64_t)timestamp.tv_sec * UINT64_C(1000000000) +
        (uint64_t)timestamp.tv_nsec;
    put_u64_be(frame + 12, monotonic_ns);
    put_u32_be(frame + 20, (uint32_t)getpid());

    if (write_all(STDOUT_FILENO, frame, sizeof(frame)) != 0) {
        return 71;
    }

    for (;;) {
        pause();
    }
}

