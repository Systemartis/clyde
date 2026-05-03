# Domain Spec: Project

## Entity: Project

A Project is identified by its absolute working directory path. All Sessions discovered for a Project originate from that Project's working directory context.

### Properties

- **cwd** (string): Absolute path to the project's working directory.

### Invariants

- Project paths MUST be absolute (relative paths are rejected with panic/error).
- Each Project is identified uniquely by its cwd.
- Two Projects with the same cwd are considered identical.

### Constructor

**New**(cwd string) Project

Constructs a new Project. Panics if cwd is not absolute.

### Method

**CWD()** string

Returns the project's absolute working directory.

## Out of Scope for V1

- Multi-project daemon or split (deferred to future architecture)
- Project metadata beyond cwd (deferred to future changes)
