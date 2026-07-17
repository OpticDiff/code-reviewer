; TypeScript symbol extraction queries
(function_declaration
  name: (identifier) @symbol.function)

(class_declaration
  name: (type_identifier) @symbol.class)

(interface_declaration
  name: (type_identifier) @symbol.interface)

(type_alias_declaration
  name: (type_identifier) @symbol.type)

(method_definition
  name: (property_identifier) @symbol.method)
