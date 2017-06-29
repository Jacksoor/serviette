load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "accounts.go",
        "login.go",
        "scripts.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/accountsservice/v1pb:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//restbridge/auth:go_default_library",
        "@com_github_dgrijalva_jwt_go//:go_default_library",
        "@com_github_emicklei_go_restful//:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes:go_default_library",
        "@org_golang_x_sync//errgroup:go_default_library",
    ],
)
