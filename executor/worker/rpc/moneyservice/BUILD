load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["service.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//bank/moneyservice/v1pb:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
