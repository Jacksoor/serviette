load("@io_bazel_rules_go//go:def.bzl", "go_library")

exports_files(["schema.sql"])

go_library(
    name = "go_default_library",
    srcs = ["store.go"],
    visibility = ["//visibility:public"],
    deps = [
        "@org_golang_x_crypto//bcrypt:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
