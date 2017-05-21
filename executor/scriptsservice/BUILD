load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["service.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//bank/accountsservice/v1pb:go_default_library",
        "//bank/moneyservice/v1pb:go_default_library",
        "//executor/pricing:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/worker:go_default_library",
        "//executor/worker/rpc/contextservice:go_default_library",
        "//executor/worker/rpc/moneyservice:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)
