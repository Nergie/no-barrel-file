// Import from deeply nested barrel file
import { 
  DEEPLY_NESTED_CONSTANT, 
  DeepestClass, 
  deepestFunction
} from "./level1/level2/level3/level4/final-level";
import {
  DeepType,
  DeepEnum
} from "./level1/level2/level3/deep-types";
import { createLevel2Component } from "./level1/level2/components";
import {
  level1Util,
  Level1Class
} from "./level1/utils";

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
