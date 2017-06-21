load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//discordbridge/web/handler:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_lib_pq//:go_default_library",
    ],
)

go_binary(
    name = "discordweb",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "web",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
