# Cubbix Logo

`cubbix-logo.pen` 是唯一可编辑源，只保留画板 `n0a0W`「Cubbix Logo · A1（1024，编辑母版）」。维护者已在 pen.dev 亲手调整并确认造型；路径、圆的位置和尺寸以母版为准。`dist/` 全部由脚本生成，不手改；`rounds/` 是冻结的评审历史，旧 SVG 和各轮推荐不作为当前源。

![Cubbix 应用图标](dist/app-icon-128.png)

## 已确认的规范

| 用途 | 规范 |
| --- | --- |
| 应用图标 | 1024×1024 透明画布；824×824 超椭圆（n=5），中心 (512,512)，每边 100px 透明边距；底色 `#FF9A3D`，猫标 `#202522` |
| 猫标布局 | 高 560px，水平中心 x=512，视觉中心 y=500；只对完整标志统一缩放和平移 |
| 菜单栏 template | 黑色与透明，18×18pt；1× 为 18px，2× 为 36px；猫标高 16pt，中心 (9,9)；去掉两枚瞳孔，保留眼洞及圆爪 |
| 独立标志 | 紧贴实际矢量边界的 SVG；`mark.svg` 为墨色，`mark-inverse.svg` 为白色，均透明背景 |

应用 PNG 尺寸为 16、32、64、128、256、512、1024px。小尺寸由 1024px 使用 Lanczos 缩小，不另改猫标。菜单栏预览各含 1×/2× 浅、深背景：浅色用黑色，深色用白色模拟系统 tint；实际 template 始终是黑色与透明。这里只检查静态导出物，应用接入留给 #8。

## 修改母版

1. 在 pen.dev 打开 `brand/cubbix-logo.pen`，修改唯一母版并保存；不要添加评审画板或从历史 SVG 覆盖母版。
2. 保留独立 path / ellipse 图层和猫头内的两个眼洞。导出器保留原始 path 字符串、viewBox 到图层尺寸的映射及图层顺序，不拟合或重画母版。
3. 运行导出和检查，复核所有尺寸、透明边距、浅深菜单栏及独立 SVG，再提交母版、脚本与生成物。

当前圆爪沿用旧的“瞳孔”图层名。导出器根据猫头眼洞范围识别真正的两枚瞳孔，不按图层名批量删除；匹配含糊或出现不支持的样式时停止导出。改动超出当前直接 path / ellipse 结构时，先扩展导出器支持，再生成产物。

## 安装与重新导出

依赖全部装在仓库外。已验证 Python 3.13、pen CLI 0.3.12、Node.js，以及以下固定渲染依赖。先按 pen.dev 官方安装说明准备 `pen`，确保 `pen version` 可运行；CLI 通过 headless interactive 的 Get/Print 读取保存后的文档，不保存或改写源。

在仓库根目录执行（`mktemp` 默认创建仓库外临时目录）：

```sh
logo_env=$(mktemp -d)
python3.13 -m venv "$logo_env/venv"
"$logo_env/venv/bin/python" -m pip install -r brand/scripts/requirements.txt
npm install --prefix "$logo_env/render" --no-audit --no-fund @resvg/resvg-js@2.6.2
export NODE_PATH="$logo_env/render/node_modules"
"$logo_env/venv/bin/python" brand/scripts/export.py --work-dir "$logo_env"
"$logo_env/venv/bin/python" brand/scripts/export.py --check
rm -rf "$logo_env"
```

超椭圆按 n=5 方程采样 4096 点。resvg 栅格化派生 SVG，Pillow 统一写 RGBA PNG 并清除元数据；过程中只在仓库外写临时文件。导出器核对读取前后的母版 SHA-256，不修改 `.pen`。如果编辑器在导出过程中保存了母版，应重新导出。

`dist/manifest.json` 记录 15 个导出文件的 SHA-256、字节数和 PNG 尺寸，以及母版、导出器的 SHA-256、画板 ID、图层数据摘要、瞳孔 ID、布局规范与渲染依赖版本。清单没有时间戳或本机路径；同一母版、脚本与依赖下重复导出应逐字节一致。

`--check` 不写文件，检查源及导出器是否过期、文件是否缺失或多出、每个文件的哈希和尺寸、1024px 图标透明边距，以及 18/36px 模板的黑色与透明约束。它验证交付完整性；更新造型后仍需人工复核外观。

定稿演变与验收记录见 [FINAL-2026-10-28.md](rounds/FINAL-2026-10-28.md)。
