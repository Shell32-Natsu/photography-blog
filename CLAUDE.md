# 协作约定

## 提交与推送

- 改动直接提交到 `master` 并推送，不要另开分支，也不要开 PR。
- 推送到 `master` 会触发自动部署，所以推送前先在本地跑过检查，推送后看一眼工作流的结果：
  - 任何推送都会重新构建站点（`.github/workflows/go.yml`）。
  - `admin/` 有改动时，还会测试并部署管理后台（`.github/workflows/admin.yml`）。
