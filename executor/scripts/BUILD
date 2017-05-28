load("@io_bazel_rules_go//go:def.bzl", "go_library")

exports_files(["schema.sql"])

go_library(
    name = "go_default_library",
    srcs = [
        "mounter.go",
        "script.go",
        "store.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/scriptsservice/v1pb:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_golang_protobuf//proto:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
