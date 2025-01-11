# Contributing to TelDrive

This guide will help you get started with contributing to TelDrive.

## Development Setup

### Prerequisites

- Go (1.19 or later)
- Node.js (for semver dependency)
- Git
- Make
- PowerShell (for Windows) or Bash (for Unix-like systems)

### Initial Setup

1. Clone the repository:
```bash
git clone https://github.com/tgdrive/teldrive.git
cd teldrive
```

2. Install dependencies:
```bash
make deps
```

## Building TelDrive

### Complete Build
To build both frontend and backend:
```bash
make build
```

### Frontend Development
The frontend is managed in a separate repository ([teldrive-ui](https://github.com/tgdrive/teldrive-ui)). The main repository pulls the latest frontend release during build.

To set up the frontend:
```bash
make frontend
```

### Backend Development
To build the backend only:
```bash
make backend
```

### Running TelDrive
After building, run the application:
```bash
make run
```

## Feature Development

1. Create a new branch for your feature:
```bash
git checkout -b feature/your-feature-name
```

2. Generate API Spec:
```bash
make gen
```

## Version Management

We follow semantic versioning (MAJOR.MINOR.PATCH):

- For bug fixes:
```bash
make patch-version
```

- For new features:
```bash
make minor-version
```

- For breaking changes:
```bash
make major-version
```

## Pull Request Guidelines

1. **Branch Naming**:
   - `feature/` for new features
   - `fix/` for bug fixes
   - `docs/` for documentation changes
   - `refactor/` for code refactoring

2. **Commit Messages**:
   - Use clear, descriptive commit messages
   - Reference issues when applicable

3. **Pull Request Description**:
   - Describe the changes made
   - Include any relevant issue numbers
   - List any breaking changes