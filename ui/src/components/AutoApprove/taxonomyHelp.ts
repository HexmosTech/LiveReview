// Mirrors internal/prompts/templates.go's TaxonomyClassificationRules — the
// one closed set of 10 finding categories the review LLM must classify every
// comment into. Keys here match internal/riskscore's FeatureSpecs category
// tags (see internal/riskscore/features.go), so they line up 1:1 with the
// risk-model override sliders in AutoApprovePolicyTab.tsx.

export interface TaxonomyCategory {
  key: string;
  label: string;
  subcategories: string[];
}

export const TAXONOMY_CATEGORIES: TaxonomyCategory[] = [
  {
    key: 'security',
    label: 'Security',
    subcategories: [
      'Authentication', 'Authorization', 'Secrets Management', 'Input Validation',
      'Injection Vulnerabilities', 'Cryptography', 'Dependency Vulnerabilities',
      'Data Exposure', 'Session Management', 'Security Logging & Auditing',
    ],
  },
  {
    key: 'reliability',
    label: 'Reliability',
    subcategories: [
      'Error Handling', 'Fault Tolerance', 'Retry Logic', 'Timeout Management',
      'Resilience Patterns', 'Availability Risks', 'Data Integrity', 'Race Conditions',
      'Resource Cleanup', 'Failure Recovery',
    ],
  },
  {
    key: 'correctness',
    label: 'Correctness',
    subcategories: [
      'Logic Errors', 'Edge Cases', 'Data Validation', 'State Management',
      'Concurrency Bugs', 'Business Rule Violations', 'Numerical Accuracy',
      'Null Handling', 'Type Safety', 'API Contract Violations',
    ],
  },
  {
    key: 'performance',
    label: 'Performance',
    subcategories: [
      'Database Efficiency', 'Algorithmic Complexity', 'Memory Usage', 'CPU Utilization',
      'Network Efficiency', 'Caching', 'Concurrency', 'Resource Contention',
      'Rendering Performance', 'Startup Performance',
    ],
  },
  {
    key: 'scalability',
    label: 'Scalability',
    subcategories: [
      'Horizontal Scaling', 'Vertical Scaling', 'Distributed Systems', 'Load Balancing',
      'Capacity Planning', 'Bottleneck Risks', 'Concurrency Limits',
      'Service Growth Constraints', 'Database Scaling', 'Queue Backpressure',
    ],
  },
  {
    key: 'compliance_governance',
    label: 'Compliance & Governance',
    subcategories: [
      'Privacy', 'Regulatory Compliance', 'Auditability', 'Data Retention',
      'Data Residency', 'Licensing', 'Policy Enforcement', 'Access Controls',
      'Change Management', 'Governance Standards',
    ],
  },
  {
    key: 'maintainability',
    label: 'Maintainability',
    subcategories: [
      'Code Complexity', 'Readability', 'Documentation', 'Code Duplication', 'Dead Code',
      'Naming Quality', 'Testability', 'Technical Debt', 'Refactoring Opportunities',
      'Configuration Management', 'UI/UX', 'Accessibility',
    ],
  },
  {
    key: 'architecture',
    label: 'Architecture',
    subcategories: [
      'Separation of Concerns', 'Modularity', 'Coupling', 'Cohesion',
      'Layering Violations', 'Dependency Management', 'Service Boundaries',
      'Domain Modeling', 'API Design', 'Extensibility',
    ],
  },
  {
    key: 'developer_experience',
    label: 'Developer Experience',
    subcategories: [
      'Testing', 'CI/CD', 'Build System', 'Local Development', 'Debuggability',
      'Observability', 'Deployment Process', 'Automation', 'Developer Tooling',
      'Documentation Quality', 'UI/UX', 'Accessibility',
    ],
  },
  {
    key: 'cost',
    label: 'Cost',
    subcategories: [
      'Cloud Resource Waste', 'Infrastructure Overprovisioning', 'Storage Optimization',
      'Database Cost Optimization', 'Excessive API Usage', 'Third-Party Service Costs',
      'Redundant Computation', 'LLM Token Consumption', 'Caching Opportunities',
      'Data Transfer Costs',
    ],
  },
];

// A short, Simplified-Technical-English hint for a category slider's hover
// help: what it does, then its sub-issues as a bulleted list (one per line,
// via InfoHint's whitespace-pre-line rendering) rather than one long
// comma-separated sentence that's hard to scan.
export const categoryHint = (cat: TaxonomyCategory): string =>
  `This slider sets the weight of "${cat.label}" findings in the risk score.\n\n` +
  `A higher value makes the engine treat these findings as riskier.\n\n` +
  `It covers:\n${cat.subcategories.map((s) => `• ${s}`).join('\n')}`;
