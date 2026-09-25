# TypeScript Review Checklist

## Focus
- **Security**: XSS (`innerHTML`, `dangerouslySetInnerHTML`, `eval`), prototype pollution.
- **Promises**: Unhandled promise rejections.
- **React**: React Hook rules (dependency arrays, side effects in render).
- **Typing**: Excessive `any` types.

## DO NOT Flag
- Formatting (handled by `prettier`).
- Import ordering (handled by `eslint-plugin-import`).
- Naming conventions unless misleading.
