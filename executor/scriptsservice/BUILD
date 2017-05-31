load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["service.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//bank/accountsservice/v1pb:go_default_library",
        "//bank/moneyservice/v1pb:go_default_library",
        "//executor/bridgeservice/v1pb:go_default_library",
        "//executor/scripts:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/worker:go_default_library",
        "//executor/worker/rpc/accountsservice:go_default_library",
        "//executor/worker/rpc/bridgeservice:go_default_library",
        "//executor/worker/rpc/moneyservice:go_default_library",
        "//executor/worker/rpc/outputservice:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_golang_protobuf//jsonpb:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
