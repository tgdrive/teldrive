<script setup lang="ts">
import { ApiReference } from '@scalar/api-reference'
import '@scalar/api-reference/style.css'
import specContent from '../../../src/openapi/openapi.yaml?raw'
</script>

<template>
  <div class="td-api-page">
    <ApiReference
      :configuration="{
        spec: {
          content: specContent,
        },
      }"
    />
  </div>
</template>
