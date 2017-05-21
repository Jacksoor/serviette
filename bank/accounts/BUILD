load("@io_bazel_rules_go//go:def.bzl", "go_library")

exports_files(["schema.sql"])

go_library(
    name = "go_default_library",
    srcs = [
        "account.go",
        "name.go",
        "store.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//bank/namesservice/v1pb:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
