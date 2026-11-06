# 同济大学

Coursive 的来源插件：用同济统一身份认证登录，读取录播课程平台上的课程、录播视频、课件和幻灯片截图，以及 Canvas 上的课程、作业、公告和文件。

## 登录

在 Coursive 的「来源」中填写学号和密码。需要验证码时，插件会显示验证码图片。

- 密码保存在 Coursive 守护进程的加密保管库中，插件本身不写任何文件。
- 令牌快过期时，插件先用单点登录会话静默续期，再用保存的密码重新登录；都失败时，「来源」会提示重新登录。
- Canvas 通过 OpenID Connect 复用同一个统一身份认证会话。

## 访问的网站

`look.tongji.edu.cn`、`iam.tongji.edu.cn`、`canvas.tongji.edu.cn`，以及课程平台返回的视频与图片地址。

## 构建

```sh
CGO_ENABLED=0 go build -trimpath -o plugins/tongji/bin/provider-linux-x64 ./plugins/tongji
```
