# no-heavyweight-jvm-frameworks

## Focus
Heavyweight runtime dependency-injection and reflection frameworks—specifically
Spring Boot / Spring Framework, Micronaut, and Quarkus—are strictly prohibited in
Azra JVM services and workers.

The platform standard for JVM services is JDK 25 virtual threads with platform-jvm-sdk
(gRPC-Netty, HikariCP, OpenTelemetry, Logback JSON) compiled with GraalVM Native Image.
Spring, Micronaut, and Quarkus introduce heavy classpath scanning, dynamic proxies,
and reflection bloat that defeat native compilation, introduce seconds of cold-start
latency, and bypass or conflict with platform tenant transaction scoping (tenantTx { ... }).

Flagged violations:
- Spring: dependencies matching org.springframework.*, spring-boot*, or annotations like
  @SpringBootApplication, @RestController, @Autowired, @Service, @Component, @Repository.
- Micronaut: dependencies matching io.micronaut.*, or annotations like @Micronaut, @Controller.
- Quarkus: dependencies matching io.quarkus.*, or annotations like @QuarkusMain, @ApplicationScoped.

## DO NOT Flag
- Remove prohibited framework dependencies and plugins from build.gradle.kts / pom.xml.
- Use pure constructor injection and manual composition roots in Main.kt.
- For data access, use platform-jvm-sdk's tenantTx { ... } with JDBC / jOOQ.
- For configuration, bind environment variables to typed data classes using platform-jvm-sdk.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/jvm-runtime
