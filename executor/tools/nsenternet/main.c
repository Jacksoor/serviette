#define _GNU_SOURCE

#include <err.h>
#include <errno.h>
#include <fcntl.h>
#include <sched.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

static char netns_path[] = "/run/netns/kobun4";

int main(int argc, char* argv[], char *env[]) {
    if (argc <= 1) {
        errx(EXIT_FAILURE, "not enough args");
    }

    int fd = open(netns_path, O_RDONLY | O_CLOEXEC);
    if (fd == -1) {
        err(EXIT_FAILURE, "open");
    }

    if (setns(fd, CLONE_NEWNET) == -1) {
        err(EXIT_FAILURE, "setns");
    }

    if (close(fd) == -1) {
        err(EXIT_FAILURE, "close");
    }

    if (setuid(getuid()) == -1) {
        err(EXIT_FAILURE, "setuid");
    }

    if (setgid(getgid()) == -1) {
        err(EXIT_FAILURE, "setgid");
    }

    if (execve(argv[1], &argv[1], env) == -1) {
        err(EXIT_FAILURE, "execve");
    }

    errx(EXIT_FAILURE, "unreachable");
}
