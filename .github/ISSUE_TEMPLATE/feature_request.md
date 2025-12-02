---
name: Feature request
about: Suggest an idea for streamz
title: '[FEATURE] '
labels: 'enhancement'
assignees: ''

---

**Is your feature request related to a problem? Please describe.**
A clear and concise description of what the problem is. Ex. I'm always frustrated when [...]

**Describe the solution you'd like**
A clear and concise description of what you want to happen.

**Describe alternatives you've considered**
A clear and concise description of any alternative solutions or features you've considered.

**Example usage**
```go
// Show how the feature would be used
ctx := context.Background()
input := make(chan streamz.Result[MyType])

// Your proposed usage
output := streamz.NewFeature[MyType](...).Process(ctx, input)
```

**Additional context**
Add any other context, diagrams, or screenshots about the feature request here.
