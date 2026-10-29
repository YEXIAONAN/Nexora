# ADR 0003：前端技术栈与契约优先

- 状态：已接受
- 日期：2026-10-29

## 决策

- Vue 3 + `<script setup>` + TypeScript 严格模式。
- Vite 7 构建，Pinia 管理跨页面状态，Vue Router 管理路由。
- Tailwind CSS 4，设计令牌通过 CSS 变量映射，支持 class 驱动的深浅色。
- API 类型由 OpenAPI 生成（`openapi-typescript`），提交生成结果。
- Vitest + Vue Test Utils 覆盖 store 与关键组件。

## 理由

- 契约优先避免前后端字段漂移。
- CSS 变量让深浅色与语义色统一，避免组件内散落颜色。
- 不引入 UI 组件库，避免模板化外观与难以控制的样式覆盖。

## 后果

- 每次契约变更需重新生成类型并提交。
- 组件与样式需要自行维护，测试覆盖关键交互。
