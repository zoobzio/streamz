# FromChainable Adapter Requirements

**Strategic Mission:** Bridge pipz and streamz Ecosystems  
**Author:** zidgel (Strategic Requirements)  
**Date:** 2025-08-24  
**Version:** 1.0

---

## Executive Summary

The FromChainable adapter represents a critical strategic capability to unify the pipz and streamz ecosystem, enabling organizations to leverage the battle-tested reliability and performance of pipz processors within streaming contexts. This feature will unlock significant business value by allowing teams to reuse existing pipz-based business logic in streaming applications without rewriting core processing components.

**Business Impact:** Teams can immediately integrate their existing, production-tested pipz processors into streaming pipelines, reducing development time, minimizing risk, and maximizing return on investment in existing processing logic.

**Current State:** The streamz README promises this functionality with code examples, but the feature does not exist. Users attempting to use `FromChainable(validator).Process(ctx, orderStream)` will encounter compilation errors.

---

## Problem Statement

### Business Problem
Organizations have invested significant engineering effort in building reliable, well-tested business logic using pipz processors. When these same organizations need to implement streaming applications, they face a critical choice:

1. **Rewrite all logic for streamz** - High risk, duplicated effort, potential for introducing bugs
2. **Use only pipz for everything** - Miss out on streaming capabilities like batching, windowing, backpressure
3. **Maintain two separate codebases** - Double the maintenance burden, inconsistent business rules

None of these options are acceptable for production systems where reliability and maintainability are paramount.

### Technical Problem  
pipz and streamz operate on fundamentally different processing models:

- **pipz**: Synchronous, single-item processing with rich error context (`T -> (T, error)`)
- **streamz**: Asynchronous, channel-based streaming with flow control (`<-chan T -> <-chan T`)

The missing FromChainable adapter prevents seamless integration between these complementary approaches, forcing organizations to choose one or the other instead of leveraging the strengths of both.

---

## Strategic Outcomes

### Primary Business Outcomes

**Integration Success:** Teams can use existing pipz processors in streaming contexts without modification
- **Measure:** Zero code changes required to existing, working pipz processors
- **Success:** Compilation and execution success for all existing pipz processors in streaming contexts

**Development Velocity:** Reduced time-to-market for streaming features using existing business logic
- **Measure:** Development time for new streaming features vs. building from scratch
- **Target:** 50% reduction in development time when reusing existing pipz processors

**Risk Reduction:** Eliminate the need to rewrite battle-tested business logic
- **Measure:** Number of production incidents related to business logic inconsistencies
- **Target:** Zero incidents caused by logic differences between pipz and streaming implementations

**Technical Debt Reduction:** Single source of truth for business processing logic
- **Measure:** Lines of duplicated business logic code across pipz and streaming implementations  
- **Target:** Zero duplication - same processor works in both contexts

### Secondary Business Outcomes

**Team Productivity:** Engineers focus on new features instead of recreating existing logic
- **Measure:** Engineer hours spent on duplication vs. new feature development
- **Target:** 80% of processor integration effort goes to new functionality

**System Reliability:** Leverage proven, production-tested processing logic
- **Measure:** Error rates in streaming vs. equivalent pipz implementations
- **Target:** Error rates within 5% between pipz and streaming contexts

**Operational Simplicity:** Single mental model for processing logic across both systems
- **Measure:** Time to onboard new engineers to processing logic
- **Target:** 25% reduction in onboarding time with unified processing model

---

## Functional Requirements

### MUST Requirements (Core v1 Functionality)

#### FR-1: Type-Safe Integration
**Requirement:** FromChainable must preserve complete type safety across the pipz-streamz boundary
- Any `pipz.Chainable[T]` must become a valid `streamz.Processor[T, T]`
- No runtime type assertions or `interface{}` usage
- Compile-time verification of type compatibility
- Generic type parameters preserved through the conversion

#### FR-2: Error Preservation  
**Requirement:** pipz error information must be preserved and accessible in streaming context
- pipz `*Error[T]` types must be convertible to streaming error handling patterns
- Complete error context preserved: input data, path, duration, timeout/cancellation flags
- Error information must be available for debugging and monitoring

#### FR-3: Context Propagation
**Requirement:** Go context must flow correctly between pipz and streaming processors  
- Context cancellation must terminate streaming operations immediately
- Context deadlines must be respected in both processing models
- Context values (tracing, request IDs) must be preserved across the boundary

#### FR-4: API Consistency
**Requirement:** FromChainable must follow streamz API patterns exactly
- Same method signatures as other streamz processor constructors
- Consistent naming patterns with streamz ecosystem
- Same behavior for context cancellation, channel closing, error handling

### SHOULD Requirements (Enhanced Functionality)

#### FR-5: Performance Optimization
**Requirement:** Minimal performance overhead when bridging processing models
- Single-item processing overhead should be < 100ns per item
- Memory allocations should be predictable and minimal
- No goroutine leaks during normal or error conditions

#### FR-6: Error Handling Strategies
**Requirement:** Configurable error handling behavior for different use cases
- Skip-on-error mode: Continue processing when individual items fail
- Fail-fast mode: Stop entire stream when any item fails  
- Error collection mode: Continue processing but collect errors for later analysis

#### FR-7: Backpressure Behavior
**Requirement:** Proper interaction with streamz backpressure mechanisms
- Blocked downstream consumers should naturally slow upstream processing
- No unbounded memory growth when consumers are slower than producers
- Graceful handling of full channel buffers

### COULD Requirements (Future Enhancement)

#### FR-8: Monitoring Integration
**Requirement:** Observable performance and behavior in streaming context
- Processing rates, error rates, latency metrics
- Integration with streamz monitoring capabilities  
- Debugging information for performance issues

#### FR-9: Batch Processing Optimization  
**Requirement:** Efficient processing when combined with streamz batching
- Special handling when FromChainable processors are used with Batcher
- Potential batch processing optimizations for compatible processors
- Memory-efficient handling of large batches

---

## Non-Functional Requirements

### Performance Requirements

**Throughput:** FromChainable-wrapped processors must achieve ≥ 90% of native pipz performance for single-item processing
- **Rationale:** Performance loss should be minimal since users are choosing to use proven pipz logic
- **Measurement:** Benchmark comparison between direct pipz usage and FromChainable-wrapped usage

**Latency:** Additional latency from adapter must be ≤ 50ns per item under normal conditions
- **Rationale:** Overhead should be negligible compared to actual business logic processing time
- **Measurement:** Microbenchmarks measuring pure adapter overhead

**Memory:** No memory leaks during normal operation or error conditions
- **Rationale:** Long-running streaming applications cannot tolerate memory leaks
- **Measurement:** Extended stress testing with memory profiling

### Reliability Requirements

**Error Isolation:** Errors in pipz processors must not crash streaming pipeline
- **Rationale:** Streaming systems need resilience to individual item processing failures
- **Measurement:** Chaos testing with intentional processor failures

**Cancellation Behavior:** Context cancellation must cleanly terminate processing within 100ms
- **Rationale:** Graceful shutdown is critical for production streaming applications
- **Measurement:** Automated tests with context cancellation under various load conditions

**Resource Cleanup:** All goroutines and channels must be properly cleaned up
- **Rationale:** Resource leaks are unacceptable in long-running streaming applications  
- **Measurement:** Goroutine leak detection in integration tests

### Usability Requirements

**API Simplicity:** Single function call converts pipz processor to streamz processor
- **Rationale:** Complex APIs reduce adoption and increase error probability
- **Measurement:** User experience testing with new engineers

**Type Safety:** Compilation errors for incompatible type usage
- **Rationale:** Runtime errors are expensive and difficult to debug
- **Measurement:** Comprehensive type compatibility test coverage

**Error Messages:** Clear, actionable error messages for common failure modes
- **Rationale:** Debugging streaming issues is inherently complex
- **Measurement:** Error message quality review with development teams

---

## Success Criteria

### Technical Success Criteria

**Compilation Success:** All existing pipz processors compile without modification when wrapped with FromChainable
- **Test:** Automated compilation of all pipz example processors through FromChainable
- **Target:** 100% compilation success rate

**Functional Equivalence:** Results from FromChainable-wrapped processors match direct pipz execution
- **Test:** Identical inputs through pipz direct vs. FromChainable-wrapped produce identical outputs
- **Target:** Byte-for-byte identical results for deterministic processors

**Error Compatibility:** Error information from wrapped processors provides equivalent debugging capability
- **Test:** Error path, data, and context information preserved through adapter
- **Target:** All pipz error fields accessible in streaming context

### Business Success Criteria

**Developer Adoption:** Teams successfully integrate existing pipz processors into streaming applications
- **Measure:** Number of successful integrations within 30 days of release
- **Target:** At least 3 successful production integrations

**Development Time Reduction:** Measurable reduction in time to implement streaming features using existing logic
- **Measure:** Before/after comparison of streaming feature development time
- **Target:** 40% reduction in development time for features using existing processors

**Production Stability:** No reliability degradation when using FromChainable in production
- **Measure:** Error rates, performance metrics compared to pure pipz or pure streamz implementations
- **Target:** Within 5% of baseline metrics

---

## Integration Requirements

### streamz Ecosystem Integration

**Processor Interface Compliance:** FromChainable output must implement streamz.Processor[T, T] exactly
- Must work seamlessly with all other streamz processors (Batcher, Filter, Mapper, etc.)
- Must compose correctly in streamz pipelines without special handling
- Must respect streamz channel conventions and lifecycle management

**Built-in Processor Interoperability:** Must chain naturally with existing streamz processors
```go
// This composition must work flawlessly
orders := make(chan Order)
validated := FromChainable(pipzValidator).Process(ctx, orders)  // pipz processor
batched := NewBatcher[Order](config).Process(ctx, validated)   // streamz processor  
processed := FromChainable(pipzProcessor).Process(ctx, batched) // Back to pipz
final := NewMonitor[Order]().Process(ctx, processed)           // streamz monitoring
```

**Error Handling Consistency:** Error behavior must align with streamz patterns
- Failed items typically skipped in streaming context (vs. pipz fail-fast)
- Error information must be accessible through streamz error handling mechanisms
- Must not break streamz error recovery and monitoring patterns

### pipz Ecosystem Compatibility

**Zero Modification Required:** Existing pipz processors must work without any code changes
- All pipz.Chainable[T] implementations must be compatible
- No special interfaces or modifications required
- Existing processor behavior and error handling unchanged

**Rich Error Preservation:** pipz Error[T] information must be fully accessible
- Complete error context: Path, InputData, Duration, Timeout, Canceled flags
- Error wrapping and unwrapping must work correctly
- Debugging information must be equivalent to direct pipz usage

---

## Scope Definition

### Explicitly IN SCOPE (v1 Deliverables)

**Core Adapter Implementation**
- FromChainable function that converts pipz.Chainable[T] to streamz.Processor[T, T]
- Type-safe conversion with full generic support
- Context propagation and error preservation
- Basic error handling (skip-on-error behavior)

**API Integration**
- Function signature matching streamz conventions
- Integration with streamz processor composition patterns
- Documentation with clear examples

**Testing Infrastructure**  
- Unit tests covering all common pipz processor types
- Integration tests with streamz pipeline compositions
- Error handling and edge case coverage
- Performance benchmarks vs. direct pipz usage

### Explicitly OUT OF SCOPE (Future Versions)

**Reverse Adaptation:** streamz.Processor to pipz.Chainable conversion
- Complexity: Streaming processors may not have single-item semantics
- Timeline: Potential v2 feature if demand exists
- Workaround: Use pipz patterns for single-item processing

**Batch Processing Optimizations:** Special handling for batch-compatible processors
- Complexity: Requires analysis of processor internals
- Timeline: Performance optimization for v3+
- Workaround: Standard item-by-item processing initially

**Advanced Error Strategies:** Complex error handling modes beyond skip-on-error
- Complexity: Requires extensive configuration options
- Timeline: v2 feature based on user feedback
- Workaround: Use streamz error handling processors downstream

**Performance Optimizations:** Zero-allocation or batch-processing modes
- Complexity: Requires deep integration with pipz internals
- Timeline: Performance optimization for v2+
- Workaround: Accept minimal overhead for v1 simplicity

---

## Architecture Constraints

### Design Constraints

**No Breaking Changes:** Implementation must not break existing streamz or pipz APIs
- Cannot modify existing streamz.Processor interface
- Cannot require changes to existing pipz.Chainable implementations
- Must maintain backward compatibility for all existing code

**Type Safety First:** No runtime type assertions or reflection
- All type conversion must be compile-time verified
- Generic type parameters must be preserved through conversion
- Type mismatches must be compilation errors, not runtime panics

**Context-First Design:** Go context must be the primary cancellation and control mechanism
- Context cancellation must immediately stop processing
- Context deadlines must be respected throughout the pipeline
- Context values must flow correctly between processing models

### Technical Constraints

**Memory Management:** Predictable memory usage without leaks
- All goroutines must have clear lifecycle management
- Channel buffers must be bounded and configurable
- No unbounded memory growth under normal or error conditions

**Error Model Compatibility:** Must bridge different error handling philosophies
- pipz: Rich error context with fail-fast behavior
- streamz: Error recovery with continue-on-failure patterns
- Conversion must preserve error information while adapting behavior

**Performance Baseline:** Must not introduce significant overhead
- Target: < 100ns overhead per item processed
- Memory: < 100 bytes additional allocation per conversion
- Latency: Streaming latency characteristics preserved

---

## Delegation and Technical Decisions

### Delegated to fidgel (Intelligence Analysis)
**Strategic Analysis Required:**
- "fidgel, analyze the broader ecosystem implications of this adapter"
- "What integration opportunities beyond basic FromChainable conversion should we consider?"
- "What patterns of usage will emerge and how can we optimize for them?"
- "Are there hidden complexities or risks in bridging these two processing models?"
- "What monitoring and observability capabilities will teams need when using this adapter?"

### Delegated to midgel (Architecture Design)
**Technical Architecture Required:**
- "midgel, design the internal architecture for FromChainable adapter implementation"
- "How should we handle the impedance mismatch between sync and async processing models?"
- "What's the optimal goroutine and channel management strategy?"
- "How do we preserve pipz error context while adapting to streamz error handling patterns?"
- "What testing strategy will verify correctness and performance requirements?"

### Delegated to kevin (Implementation)
**Implementation Planning Required:**
- "kevin, implement the FromChainable adapter based on midgel's architecture design"
- "Create comprehensive test coverage for all pipz processor types"
- "Implement performance benchmarks comparing direct pipz vs. FromChainable usage"
- "Document usage patterns and integration examples"
- "Verify all requirements are met and no regressions introduced"

---

## Open Questions Requiring Clarification

### Business Requirements
1. **Error Handling Strategy:** Should default behavior be skip-on-error or fail-fast for streaming context?
   - **Impact:** Affects reliability patterns and user expectations
   - **Stakeholder:** Product team and early adopters
   - **Decision Required:** Before architecture design begins

2. **Performance Trade-offs:** What performance overhead is acceptable for the convenience of reusing pipz processors?
   - **Impact:** Determines implementation complexity and optimization requirements
   - **Stakeholder:** Engineering teams and performance-sensitive use cases
   - **Decision Required:** Before implementation approach is finalized

### Technical Requirements  
3. **Context Timeout Behavior:** How should context deadlines interact with streaming processors that may buffer items?
   - **Impact:** Affects reliability and predictability of timeouts
   - **Stakeholder:** Operations teams managing production systems
   - **Decision Required:** During architecture design phase

4. **Memory Management:** What's the acceptable memory overhead for the adapter layer?
   - **Impact:** Affects scalability for high-throughput applications
   - **Stakeholder:** Platform teams managing resource utilization
   - **Decision Required:** Before performance optimization decisions

### Integration Requirements
5. **Error Information Access:** How should streaming applications access rich pipz error context for debugging?
   - **Impact:** Affects debugging capabilities and operational monitoring
   - **Stakeholder:** SRE teams and application developers
   - **Decision Required:** During API design phase

---

## Risk Assessment

### High-Impact Risks

**Risk:** Performance overhead makes FromChainable unusable for high-throughput scenarios
- **Probability:** Medium - Channel overhead inherent in streaming model
- **Impact:** High - Core value proposition compromised
- **Mitigation:** Early performance benchmarking and optimization focus

**Risk:** Error handling mismatch causes silent failures or debugging difficulties
- **Probability:** Medium - Different error philosophies between pipz and streamz
- **Impact:** High - Production reliability concerns
- **Mitigation:** Comprehensive error handling tests and clear documentation

**Risk:** Complex edge cases in context cancellation or timeout handling  
- **Probability:** Medium - Interactions between sync and async cancellation
- **Impact:** Medium - Operational reliability in production
- **Mitigation:** Extensive integration testing with chaos engineering

### Medium-Impact Risks

**Risk:** API design doesn't align well with streamz patterns, causing adoption friction
- **Probability:** Low - Can be addressed through design review
- **Impact:** Medium - Reduced adoption and developer satisfaction
- **Mitigation:** Early API review with streamz users

**Risk:** Memory leaks under specific error conditions or usage patterns
- **Probability:** Low - Preventable through testing
- **Impact:** Medium - Production stability issues
- **Mitigation:** Extended stress testing and memory profiling

---

## Success Metrics and Monitoring

### Development Phase Metrics
- **API Design Quality:** Time for new developers to successfully use FromChainable
- **Implementation Coverage:** Percentage of pipz processor types successfully converted
- **Performance Achievement:** Actual vs. target performance overhead measurements

### Production Adoption Metrics  
- **Usage Growth:** Number of FromChainable integrations in production over time
- **Error Rates:** Production error rates comparing direct pipz vs. FromChainable usage
- **Performance Impact:** Latency and throughput measurements in production streaming applications

### Business Value Metrics
- **Development Velocity:** Time saved by reusing existing pipz processors vs. rewriting
- **Code Reuse:** Percentage of business logic that can be shared between pipz and streaming contexts
- **Risk Reduction:** Number of bugs avoided by reusing battle-tested processors

---

## Conclusion

The FromChainable adapter represents a strategic capability that will unlock significant business value by bridging the pipz and streamz ecosystems. By enabling seamless reuse of existing, production-tested business logic in streaming contexts, this feature reduces development time, minimizes risk, and maximizes return on investment in existing processing infrastructure.

Success requires careful attention to performance, reliability, and usability requirements while maintaining the type safety and composability that makes both pipz and streamz valuable. The technical challenges are solvable with proper architecture and implementation, but business requirements must be clearly defined before technical work begins.

**Key Success Factors:**
1. **Performance First:** Minimal overhead ensures broad adoption
2. **Type Safety:** Compile-time verification prevents runtime issues  
3. **Error Transparency:** Rich debugging information maintains operational excellence
4. **API Consistency:** Seamless integration with existing streamz patterns

This requirements document provides the strategic foundation for fidgel's analysis, midgel's architecture design, and kevin's implementation work that will bring this critical capability to production.

---

*Next Actions:*
1. **fidgel:** Review and analyze broader ecosystem implications and opportunities
2. **midgel:** Design architecture based on these requirements and fidgel's analysis
3. **kevin:** Implement based on midgel's architecture design
4. **Stakeholder review:** Clarify open questions about error handling and performance trade-offs