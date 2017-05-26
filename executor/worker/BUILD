load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "supervisor.go",
        "worker.go",
    ],
    visibility = ["//visibility:public"],
    deps = ["@com_github_djherbis_buffer//limio:go_default_library"],
)
