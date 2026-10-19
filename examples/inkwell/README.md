# Inkwell

Inkwell is a small made-up publishing app. It is a safe place to try Dummy Run before you point it at your own
product. Everything in it is fake, and it keeps its data in memory.

```sh
go run ./examples/inkwell -test-login
```

The app listens on `http://127.0.0.1:8080`. The [Inkwell guide](../../docs/INKWELL.md) covers the roles, the test
accounts, the planted-defect manifest and how to run a persona against it.
