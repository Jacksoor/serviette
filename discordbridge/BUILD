load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

exports_files(["schema.sql"])

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//discordbridge/client:go_default_library",
        "//discordbridge/messagingservice:go_default_library",
        "//discordbridge/networkinfoservice:go_default_library",
        "//discordbridge/statsservice:go_default_library",
        "//discordbridge/statsstore:go_default_library",
        "//discordbridge/varstore:go_default_library",
        "//executor/messagingservice/v1pb:go_default_library",
        "//executor/networkinfoservice/v1pb:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/statsservice/v1pb:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_lib_pq//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//grpclog/glogger:go_default_library",
        "@org_golang_google_grpc//reflection:go_default_library",
        "@org_golang_x_net//trace:go_default_library",
    ],
)

go_binary(
    name = "discordbridge",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
