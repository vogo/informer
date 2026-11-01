//go:build darwin && cgo

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

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Foundation/Foundation.h>

static id attr(AXUIElementRef element, CFStringRef name) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, name, &value) != kAXErrorSuccess) return nil;
    return [(id)value autorelease];
}
static NSString *textAttr(AXUIElementRef e, CFStringRef key) {
    id v = attr(e, key);
    return [v isKindOfClass:[NSString class]] ? v : @"";
}
static NSArray *children(AXUIElementRef e) {
    id v = attr(e, kAXChildrenAttribute);
    return [v isKindOfClass:[NSArray class]] ? v : @[];
}
static NSDictionary *identity(AXUIElementRef e) {
    return @{@"role": textAttr(e, kAXRoleAttribute), @"title": textAttr(e, kAXTitleAttribute),
             @"description": textAttr(e, kAXDescriptionAttribute), @"identifier": textAttr(e, CFSTR("AXIdentifier"))};
}
static BOOL walk(AXUIElementRef e, NSArray *path, NSInteger parent, NSMutableArray *nodes, int depth) {
    if (depth > 40 || nodes.count >= 6000) return NO;
    NSMutableDictionary *node = [[identity(e) mutableCopy] autorelease];
    node[@"path"] = path;
    node[@"parent"] = @(parent);
    node[@"value"] = textAttr(e, kAXValueAttribute);
    node[@"enabled"] = attr(e, kAXEnabledAttribute) ?: @YES;
    Boolean settable = false;
    AXUIElementIsAttributeSettable(e, kAXValueAttribute, &settable);
    node[@"writable"] = @(settable);
    CFArrayRef actions = NULL;
    if (AXUIElementCopyActionNames(e, &actions) == kAXErrorSuccess) {
        node[@"pressable"] = @([(NSArray *)actions containsObject:(NSString *)kAXPressAction]);
        CFRelease(actions);
    }
    NSInteger index = nodes.count;
    [nodes addObject:node];
    NSArray *list = children(e);
    for (NSUInteger i = 0; i < list.count; i++) {
        if (!walk((AXUIElementRef)list[i], [path arrayByAddingObject:@(i)], index, nodes, depth + 1)) return NO;
    }
    return YES;
}
// Revalidate the chat and exact draft immediately before the sending action.
static BOOL sendPreconditions(AXUIElementRef window, NSDictionary *r) {
    NSMutableArray *nodes = [NSMutableArray array];
    if (!walk(window, @[], -1, nodes, 0)) return NO;
    NSString *group = r[@"group"], *body = r[@"body"];
    if (!group.length || !body.length) return NO;
    BOOL header = NO;
    NSUInteger composers = 0;
    for (NSUInteger i = 0; i < nodes.count; i++) {
        NSDictionary *n = nodes[i];
        NSString *role = n[@"role"];
        BOOL search = [role isEqual:@"AXSearchField"] || [n[@"title"] isEqual:@"搜索"] || [n[@"description"] isEqual:@"搜索"] ||
            [n[@"title"] isEqual:@"Search"] || [n[@"description"] isEqual:@"Search"];
        if ([n[@"writable"] boolValue] && !search && ([role isEqual:@"AXTextArea"] || [role isEqual:@"AXTextField"])) {
            composers++;
            if (![n[@"value"] isEqual:body]) return NO;
        }
        if (search && [n[@"value"] length] != 0) return NO;
        BOOL history = NO;
        for (NSInteger p = [n[@"parent"] integerValue]; p >= 0 && p < nodes.count; p = [nodes[p][@"parent"] integerValue]) {
            if ([@[@"AXTable", @"AXList", @"AXRow", @"AXOutline"] containsObject:nodes[p][@"role"]]) { history = YES; break; }
        }
        if (history || ![@[@"AXStaticText", @"AXButton"] containsObject:role]) continue;
        NSString *pattern = [NSString stringWithFormat:@"^%@\\s*[（(][0-9]+[)）]$", [NSRegularExpression escapedPatternForString:group]];
        for (NSString *key in @[@"title", @"value", @"description"]) {
            NSString *value = n[key];
            if ([value isEqual:group] || [value rangeOfString:pattern options:NSRegularExpressionSearch].location != NSNotFound) header = YES;
        }
    }
    return header && composers == 1;
}
static NSDictionary *execute(NSDictionary *r) {
    if (!AXIsProcessTrusted()) return @{@"error": @"请在系统设置的隐私与安全性中，为当前运行的 informer（CLI 则为终端）开启辅助功能权限；安装或更新后可能需要重新授权"};
    NSDictionary *session = [(id)CGSessionCopyCurrentDictionary() autorelease];
    if (!session || [session[@"CGSSessionScreenIsLocked"] boolValue] || ![session[(__bridge NSString *)kCGSessionOnConsoleKey] boolValue])
        return @{@"error": @"请登录并解锁当前 macOS 桌面后再推送微信"};
    NSArray *apps = [NSRunningApplication runningApplicationsWithBundleIdentifier:@"com.tencent.xinWeChat"];
    if (apps.count != 1) return @{@"error": @"请先打开微信并登录；当前需有且仅有一个微信进程"};
    NSRunningApplication *app = apps[0];
    NSString *op = r[@"op"];
    if ([op isEqual:@"activate"]) {
        if (![app activateWithOptions:0]) return @{@"error": @"无法激活微信窗口"};
    }
    AXUIElementRef root = AXUIElementCreateApplication(app.processIdentifier);
    AXUIElementSetMessagingTimeout(root, 1.0);
    AXUIElementRef window = (AXUIElementRef)attr(root, kAXFocusedWindowAttribute);
    if (!window) { CFRelease(root); return @{@"error": @"未找到微信主窗口，请登录微信并打开聊天窗口"}; }
    if ([attr(window, kAXMinimizedAttribute) boolValue]) {
        CFRelease(root);
        return @{@"error": @"请先恢复微信聊天窗口，当前窗口已最小化"};
    }
    NSDictionary *result = nil;
    if ([op isEqual:@"status"] || [op isEqual:@"activate"]) {
        result = @{@"message": @"微信已运行，辅助功能权限正常；可定位群聊", @"window": textAttr(window, kAXTitleAttribute)};
    } else if ([op isEqual:@"snapshot"]) {
        NSMutableArray *nodes = [NSMutableArray array];
        if (!walk(window, @[], -1, nodes, 0)) result = @{@"error": @"微信界面结构过大或不受支持，请关闭聊天详情等浮窗后重试"};
        else result = @{@"nodes": nodes, @"window": textAttr(window, kAXTitleAttribute)};
    } else {
        if (!app.active) { CFRelease(root); return @{@"error": @"微信失去前台焦点，操作已停止"}; }
        AXUIElementRef target = window;
        for (NSNumber *part in r[@"node"][@"path"]) {
            NSArray *list = children(target);
            if (part.unsignedIntegerValue >= list.count) { target = NULL; break; }
            target = (AXUIElementRef)list[part.unsignedIntegerValue];
        }
        if (!target || ![identity(target) isEqual: @{ @"role": r[@"node"][@"role"] ?: @"",
                @"title": r[@"node"][@"title"] ?: @"", @"description": r[@"node"][@"description"] ?: @"",
                @"identifier": r[@"node"][@"identifier"] ?: @"" }]) {
            result = @{@"error": @"微信界面发生变化，操作已停止，请重新定位目标群"};
        } else {
            AXError error = kAXErrorFailure;
            if ([op isEqual:@"set"]) {
                if (![textAttr(target, kAXValueAttribute) isEqual:(r[@"node"][@"value"] ?: @"")]) {
                    CFRelease(root);
                    return @{@"error": @"输入框内容发生变化，已停止；不会覆盖新搜索或草稿"};
                }
                error = AXUIElementSetAttributeValue(target, kAXValueAttribute, (CFTypeRef)r[@"value"]);
            }
            if ([op isEqual:@"press"]) error = AXUIElementPerformAction(target, kAXPressAction);
            if ([op isEqual:@"send"]) {
                BOOL sendButton = [textAttr(target, kAXRoleAttribute) isEqual:@"AXButton"] &&
                    ([@[@"发送", @"Send"] containsObject:textAttr(target, kAXTitleAttribute)] ||
                     [@[@"发送", @"Send"] containsObject:textAttr(target, kAXDescriptionAttribute)]);
                if (!sendButton || !sendPreconditions(window, r) || !app.active) {
                    CFRelease(root);
                    return @{@"error": @"点击发送前群名、草稿或焦点发生变化，已停止"};
                }
                error = AXUIElementPerformAction(target, kAXPressAction);
            }
            if ([op isEqual:@"select"]) error = AXUIElementSetAttributeValue(target, kAXSelectedAttribute, kCFBooleanTrue);
            if (error == kAXErrorSuccess) result = @{@"message": @"ok"};
            else result = @{@"error": [NSString stringWithFormat:@"微信控件不支持此操作（辅助功能错误 %d），已停止", error]};
        }
    }
    CFRelease(root);
    return result ?: @{@"error": @"未知微信操作"};
}
char *informer_wechat(const char *request) {
    @autoreleasepool {
        @try {
            NSData *input = [[NSString stringWithUTF8String:request] dataUsingEncoding:NSUTF8StringEncoding];
            NSDictionary *r = [NSJSONSerialization JSONObjectWithData:input options:0 error:nil];
            NSDictionary *result = execute(r);
            NSData *data = [NSJSONSerialization dataWithJSONObject:result options:0 error:nil];
            return strdup([[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] autorelease].UTF8String);
        } @catch (NSException *e) {
            return strdup("{\"error\":\"微信辅助功能接口异常，已停止操作\"}");
        }
    }
}
