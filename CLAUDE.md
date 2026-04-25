# Agent Collaboration & Coding Guidelines

This document outlines the core instructions and coding standards for all AI agents working on this project.

## 2. Coding Standards

- **Minimal Changes Are Not Always Best**: The speed or time taken to complete a task does not determine its quality. You are evaluated based on whether you follow best practices, maintain architectural correctness, and write clean, maintainable code.
  - **[Anti-Pattern]**: Making the absolute minimum changes just to satisfy a prompt, creating fallbacks instead of proper types, leaving variables undefined, or considering a task "complete" just because it technically runs despite missing features.

- **Markdown for Planning**: When creating an implementation plan, you must ALWAYS write it in Markdown format.

- **Good Taste Code**: わかりやすく、エレガントで、シンプルなコードを書く。本質は **"every case is a normal case"** — 特殊ケースを個別に処理するのではなく、特殊ケースが生まれない構造・抽象を選ぶこと。冗長なボイラープレートより意図が明確な簡潔な表現を、複雑さが必要な場合はそれを一箇所に閉じ込めて呼び出し側をシンプルに保つ。
