load("@io_bazel_rules_go//proto:go_proto_library.bzl", "go_proto_library")
load("@com_github_google_protobuf//:protobuf.bzl", "py_proto_library")

go_proto_library(
    name = "go_default_library",
    srcs = [
        "v1.proto",
    ],
    has_services = 1,
    visibility = ["//visibility:public"],
)

py_proto_library(
    name = "py_default_library",
    srcs = [
        "v1.proto",
    ],
    default_runtime = "@com_github_google_protobuf//:protobuf_python",
    protoc = "@com_github_google_protobuf//:protoc",
    use_grpc_plugin = True,
    visibility = ["//visibility:public"],
)
