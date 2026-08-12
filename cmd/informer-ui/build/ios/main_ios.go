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

//go:build ios

package main

import (
	"C"
)

// For iOS builds we export a function the WailsAppDelegate can call.
// This wrapper keeps the user's main.go unmodified.

// WailsIOSMain runs the user's main() (application.New / app.Run). It is invoked
// by the WailsAppDelegate from didFinishLaunchingWithOptions — i.e. only AFTER
// UIKit has launched — on a BACKGROUND thread, so the Go runtime never starts
// concurrently with UIApplicationMain (that race intermittently corrupts the
// FrontBoard launch handshake on a physical device → blank cold launch /
// scene-create watchdog 0x8BADF00D). Keeping all app setup off the OS main
// thread also leaves UIApplicationMain unobstructed on the main thread.
//
//export WailsIOSMain
func WailsIOSMain() {
	main()
}
