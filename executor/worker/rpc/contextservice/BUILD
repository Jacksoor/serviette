load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["service.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/scriptsservice/v1pb:go_default_library",
        "@com_github_golang_protobuf//jsonpb:go_default_library",
    ],
)
