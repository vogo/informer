/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agent_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
)

// agentE2EEnv opts into the tests that drive a real agent command line. They are
// off by default because they take minutes and spend real api budget.
//
//	INFORMER_AGENT_E2E=1 go test ./internal/agent -run SessionContinues -v
const agentE2EEnv = "INFORMER_AGENT_E2E"

// TestSessionContinuesARealConversation is the only proof that resuming works.
//
// Every other test in this file asserts that the right flags went out, which a
// stand-in can show. Whether the model on the other end actually sees the
// earlier turn is a property of the command line, not of informer, and the only
// honest way to check it is to ask it something only the first turn could
// answer.
func TestSessionContinuesARealConversation(t *testing.T) {
	t.Parallel()

	if os.Getenv(agentE2EEnv) == "" {
		t.Skipf("set %s=1 to run the real agent conversation", agentE2EEnv)
	}

	session := agent.NewSession(&agent.Config{})

	_, err := session.Send(t.Context(), "Remember this number: 4271. Reply with just OK.", nil)
	require.NoError(t, err)

	answer, err := session.Send(t.Context(), "What number did I ask you to remember? Reply with the number alone.", nil)
	require.NoError(t, err)

	require.Contains(t, answer, "4271")
	require.Equal(t, 2, session.Turns())
}
