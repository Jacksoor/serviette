load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "bankcommands.go",
        "client.go",
        "command.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//bank/accountsservice/v1pb:go_default_library",
        "//bank/assetsservice/v1pb:go_default_library",
        "//bank/moneyservice/v1pb:go_default_library",
        "//discordbridge/store:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "@com_github_bwmarrin_discordgo//:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_hako_durafmt//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
