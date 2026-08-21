// Package version 集中定义 cm 的版本号。
//
// 这是全仓库唯一需要修改版本号的地方：Makefile 的 VERSION 与发布产物
// 文件名都通过 awk 从这里反读，`cm --version` 也直接取本常量，因此升版本
// 只需改下面这一行。
package version

// Version 当前版本。发布时同步打 git tag v<Version> 并上传 Release。
const Version = "0.3.0"
