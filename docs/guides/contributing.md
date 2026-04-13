# Contributing Guide

Thank you for your interest in contributing to AutoAnimeDownloader! This guide will help you get started.

## How to Contribute

There are many ways to contribute:

- 🐛 **Report bugs** - Help us find and fix issues
- 💡 **Suggest features** - Share your ideas
- 📝 **Improve documentation** - Make docs better
- 🔧 **Submit code** - Fix bugs or add features
- 🧪 **Write tests** - Improve test coverage
- 🌐 **Translate** - Help with internationalization

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork**:
   ```bash
   git clone https://github.com/YOUR_USERNAME/AutoAnimeDownloader.git
   cd AutoAnimeDownloader
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/icarosuper/AutoAnimeDownloader.git
   ```
4. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

See [Development Guide](development.md) for detailed setup instructions.

Quick setup:
```bash
# Install Go dependencies
go mod download

# Install frontend dependencies
cd src/internal/frontend && npm install && cd ../../..

# Build frontend (required before Go build)
cd src/internal/frontend && npm run build && cd ../../..
```

## Making Changes

### Code Style

- **Go**: Follow standard Go conventions, use `gofmt`
- **Frontend**: Use Prettier, follow Svelte best practices
- **Commits**: Write clear, descriptive commit messages

### Commit Messages

Use clear, descriptive commit messages:

```
feat: Add new endpoint for episode filtering
fix: Resolve memory leak in state manager
docs: Update installation guide
test: Add tests for config validation
refactor: Simplify daemon loop logic
```

Prefixes:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `test:` - Test additions/changes
- `refactor:` - Code refactoring
- `style:` - Code style changes
- `chore:` - Build/tooling changes

### Testing

- Write tests for new features
- Ensure all tests pass: `go test ./...`
- Run with race detector: `go test -race ./...`
- Maintain or improve test coverage

### Documentation

- Update relevant documentation
- Add comments for exported functions
- Update README if needed
- Update API documentation (Swagger)

## Submitting Changes

### 1. Keep Your Fork Updated

```bash
git fetch upstream
git checkout main
git merge upstream/main
```

### 2. Push Your Changes

```bash
git push origin feature/your-feature-name
```

### 3. Create a Pull Request

1. Go to the repository on GitHub
2. Click "New Pull Request"
3. Select your branch
4. Fill out the PR template
5. Submit the PR

### Pull Request Template

When creating a PR, include:

- **Description**: What does this PR do?
- **Type**: Bug fix, feature, documentation, etc.
- **Testing**: How was this tested?
- **Checklist**: 
  - [ ] Tests pass
  - [ ] Documentation updated
  - [ ] Code follows style guidelines
  - [ ] No breaking changes (or documented)

## Code Review Process

1. **Automated checks** run on PRs (tests, linting)
2. **Maintainers review** the code
3. **Feedback** is provided if needed
4. **Changes** are requested if necessary
5. **Approval** when ready
6. **Merge** by maintainers

## Reporting Bugs

### Before Reporting

1. Check if the bug already exists in issues
2. Try to reproduce the bug
3. Check logs for errors
4. Test with latest version

### Bug Report Template

```markdown
**Description**
Clear description of the bug

**Steps to Reproduce**
1. Step one
2. Step two
3. ...

**Expected Behavior**
What should happen

**Actual Behavior**
What actually happens

**Environment**
- OS: [e.g., Linux, Windows]
- Version: [e.g., 1.0.0]
- Go version: [e.g., 1.24]
- Node version: [e.g., 20]

**Logs**
Relevant log entries

**Additional Context**
Any other relevant information
```

## Suggesting Features

### Feature Request Template

```markdown
**Feature Description**
Clear description of the feature

**Use Case**
Why is this feature needed?

**Proposed Solution**
How should it work?

**Alternatives Considered**
Other solutions you've thought about

**Additional Context**
Any other relevant information
```

## Coding Guidelines

### Go Code

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Handle all errors
- Write tests
- Document exported functions

### Frontend Code

- Use Svelte best practices
- Use Tailwind CSS for styling
- Keep components small
- Write accessible HTML

### General

- Write clear, readable code
- Add comments for complex logic
- Keep functions focused
- Avoid code duplication

## Testing Guidelines

### Unit Tests

- Test all exported functions
- Test error cases
- Use table-driven tests when appropriate
- Mock external dependencies

### Integration Tests

- Test complete workflows
- Use mock servers for external APIs
- Test error scenarios
- Clean up after tests

## Documentation Guidelines

- Keep documentation up to date
- Use clear, simple language
- Include examples when helpful
- Update related docs when changing code

## Questions?

- Open an issue for questions
- Check existing issues/PRs
- Review documentation

## Code of Conduct

Be respectful and constructive. We're all here to make the project better.

## License

By contributing, you agree that your contributions will be licensed under the GNU General Public License v3.0 (GPL v3).

## Recognition

Contributors will be recognized in:
- README.md (if significant contributions)
- Release notes
- Project documentation

Thank you for contributing to AutoAnimeDownloader! 🎉

