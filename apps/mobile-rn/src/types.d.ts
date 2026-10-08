/**
 * Uniwind / Tailwind 的 className 由编译期转换，TS 不需要知道具体类名；
 * 这里只是为了让 `import "../global.css"` 这种副作用导入通过类型检查。
 */
declare module "*.css";