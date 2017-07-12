load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["seccomp.go"],
    visibility = ["//visibility:public"],
    deps = ["@com_github_opencontainers_runc//libcontainer/configs:go_default_library"],
)
