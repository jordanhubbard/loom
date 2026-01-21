# Persona Templates

Pre-built persona templates for common roles and use cases.

## Available Templates

### Software Development

- **backend-developer**: Backend/API development specialist
- **frontend-developer**: UI/UX and frontend specialist  
- **fullstack-developer**: Full-stack development
- **devops-engineer**: Infrastructure and deployment
- **qa-engineer**: Testing and quality assurance
- **security-engineer**: Security auditing and hardening

### Data & AI

- **data-scientist**: Data analysis and ML
- **ml-engineer**: Machine learning implementation
- **data-engineer**: Data pipelines and ETL

### Business & Product

- **product-manager**: Product strategy and roadmap
- **project-manager**: Project coordination
- **business-analyst**: Requirements and analysis
- **technical-writer**: Documentation specialist

### Support & Operations

- **support-engineer**: Customer support and troubleshooting
- **sre**: Site reliability engineering
- **database-admin**: Database management

## Usage

### From Web UI

1. Go to Persona Editor
2. Click "Load Template"
3. Select template
4. Customize as needed
5. Save

### From CLI

```bash
cp personas/templates/backend-developer.md personas/my-backend-dev.md
# Edit as needed
```

### From API

```bash
curl http://localhost:8080/api/v1/personas/templates
curl http://localhost:8080/api/v1/personas/templates/backend-developer
```

## Template Structure

Each template includes:
- **Role**: Job title/function
- **Instructions**: How to think and act
- **Capabilities**: What they can do
- **Best Practices**: Guidelines to follow
- **Example Tasks**: Common work items

## Creating Custom Templates

1. Start with existing template
2. Modify instructions and capabilities
3. Test with sample tasks
4. Save to `personas/templates/`
5. Add to template registry

## Template Guidelines

- Clear, specific instructions
- 5-10 key capabilities
- Real-world examples
- Best practices included
- Task-focused

---

**Use templates to quickly create specialized personas!**
