load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["handler.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/accounts:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@org_golang_x_net//webdav:go_default_library",
    ],
)
