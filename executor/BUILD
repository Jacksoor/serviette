load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    data = [
        "//clients",
    ],
    visibility = ["//visibility:private"],
    deps = [
        "//bank/accountsservice/v1pb:go_default_library",
        "//bank/moneyservice/v1pb:go_default_library",
        "//executor/pricing:go_default_library",
        "//executor/scripts:go_default_library",
        "//executor/scriptsservice:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/worker:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//grpclog/glogger:go_default_library",
        "@org_golang_google_grpc//reflection:go_default_library",
        "@org_golang_x_net//trace:go_default_library",
    ],
)

go_binary(
    name = "executor",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)
