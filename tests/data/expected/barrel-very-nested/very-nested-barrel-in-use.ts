// Import from deeply nested barrel file
import { DeepType, DeepEnum } from "@barrel-very-nested/level1/level2/level3/deep-types";
import { createLevel2Component } from "@barrel-very-nested/level1/level2/components";
import { level1Util, Level1Class } from "@barrel-very-nested/level1/utils";
// Importing multiple exports from the same barrel file in different ways
import { DEEPLY_NESTED_CONSTANT, DeepestClass, deepestFunction } from "@barrel-very-nested/level1/level2/level3/level4/final-level";

// Usage examples
const constant = DEEPLY_NESTED_CONSTANT;
const deepClass = new DeepestClass();
const processed = deepestFunction("test input");

const deepData: DeepType = {
  level: 3,
  nested: {
    value: "nested value",
    metadata: {
      created: new Date(),
      author: "test author"
    }
  }
};

const enumValue = DeepEnum.LEVEL_THREE;
const component = createLevel2Component("comp1", "Test Component");
const utility = level1Util();
const level1Instance = new Level1Class();
